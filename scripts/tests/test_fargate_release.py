import argparse
import importlib.util
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("fargate_release", Path(__file__).parents[1] / "fargate-release.py")
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)
SHA = "a" * 40
CONTROL_SHA = "c" * 40
STACK = "owner/ajent-release-executor/production"
URN = "urn:pulumi:production::ajent-release-executor::aws:ssm/parameter:Parameter::jitParameter"
TASK = "arn:aws:ecs:us-west-2:394824061124:task/ajent-release/abc"
OUTPUTS = {"clusterArn": "arn:aws:ecs:us-west-2:394824061124:cluster/ajent-release",
           "taskDefinitionArn": "arn:aws:ecs:us-west-2:394824061124:task-definition/ajent-release-runner:1",
           "securityGroupId": "sg-abc", "subnetId": "subnet-abc"}


def preview(urn=URN, op="update", extra=None):
    old = {"name": "/ajent/release-executor/jit", "type": "SecureString", "value": "masked-old"}
    new = dict(old, value="masked-new")
    new.update(extra or {})
    return "\n".join(json.dumps(e) for e in [
        {"resourcePreEvent": {"metadata": {"urn": urn, "type": "aws:ssm/parameter:Parameter", "op": op,
                                             "old": {"inputs": old}, "new": {"inputs": new}}}},
        {"summaryEvent": {"resourceChanges": {"same": 20, op: 1}}}])


def run(status="queued", conclusion=None, id=11):
    return {"id": id, "head_sha": CONTROL_SHA, "head_branch": "main", "event": "workflow_dispatch",
            "repository": {"full_name": release.CONTROL_REPO},
            "created_at": "2099-01-01T00:00:00Z", "status": status, "conclusion": conclusion,
            "html_url": "https://github.com/ajent-social/ajent-social/actions/runs/11"}


class AuthenticationTests(unittest.TestCase):
    def test_captured_gh_token_reaches_only_pulumi_environment(self):
        calls = []
        def subprocess_run(args, **kwargs):
            calls.append((args, kwargs))
            return SimpleNamespace(stdout="private-token\n" if args == ["gh", "auth", "token"] else "{}")
        with mock.patch.dict(os.environ, {}, clear=True), mock.patch.object(release.subprocess, "run", subprocess_run):
            release.command(["pulumi", "stack", "output"])
            release.command(["aws", "ecs", "list-clusters"])
        self.assertEqual(calls[1][1]["env"]["GITHUB_TOKEN"], "private-token")
        self.assertIsNone(calls[2][1]["env"])
        self.assertTrue(all("private-token" not in arg for args, _ in calls for arg in args))
        self.assertNotIn("GITHUB_TOKEN", os.environ)

    def test_existing_github_provider_token_is_preserved(self):
        calls = []
        def subprocess_run(args, **kwargs):
            calls.append((args, kwargs))
            return SimpleNamespace(stdout="{}")
        with mock.patch.dict(os.environ, {"GITHUB_TOKEN": "already-scoped"}, clear=True), mock.patch.object(release.subprocess, "run", subprocess_run):
            release.command(["pulumi", "preview"])
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][1]["env"]["GITHUB_TOKEN"], "already-scoped")


class PreviewTests(unittest.TestCase):
    def test_only_value_update_is_accepted(self):
        self.assertEqual(release.validate_jit_preview(preview(), STACK), 1)

    def test_unrelated_resource_and_replacement_are_rejected(self):
        for data in (preview(urn=URN.replace("jitParameter", "other")), preview(op="create"),
                     preview(op="delete"), preview(extra={"name": "/other"})):
            with self.subTest(data=data), self.assertRaises(ValueError):
                release.validate_jit_preview(data, STACK)

    def test_missing_summary_and_input_evidence_fail_closed(self):
        with self.assertRaises(ValueError):
            release.validate_jit_preview(preview().splitlines()[0], STACK)
        data = json.loads(preview().splitlines()[0])
        del data["resourcePreEvent"]["metadata"]["old"]
        with self.assertRaises(ValueError):
            release.validate_jit_preview(json.dumps(data) + "\n" + preview().splitlines()[1], STACK)

    def test_noop_is_accepted(self):
        self.assertEqual(release.validate_jit_preview(json.dumps({"summaryEvent": {"resourceChanges": {"same": 20}}}), STACK), 0)

    def test_secret_is_passed_on_stdin_and_apply_is_plan_bound(self):
        calls = []
        def command(args, **kwargs):
            calls.append((args, kwargs))
            return preview() if "preview" in args else ""
        with mock.patch.object(release, "command", command):
            release.set_jit(STACK, Path("/executor"), "private-one-use-token")
        self.assertEqual(calls[0][1]["body"], "private-one-use-token\n")
        self.assertTrue(all("private-one-use-token" not in arg for args, _ in calls for arg in args))
        self.assertIn("--plan", calls[-1][0])
        self.assertIn("--refresh=false", calls[-1][0])


class CorrelationTests(unittest.TestCase):
    def test_only_unique_new_exact_main_dispatch_matches(self):
        self.assertEqual(release.matching_dispatch([run()], {1}, CONTROL_SHA, "2026-01-01T00:00:00Z")["id"], 11)
        self.assertIsNone(release.matching_dispatch([run()], {11}, CONTROL_SHA, "2026-01-01T00:00:00Z"))
        with self.assertRaises(ValueError):
            release.matching_dispatch([run(), run(id=12)], set(), CONTROL_SHA, "2026-01-01T00:00:00Z")

    def test_preconditions_reject_active_release_or_failed_ci(self):
        def github(endpoint):
            if endpoint.endswith("commits/main"):
                return {"sha": SHA}
            if "test.yml" in endpoint:
                return {"workflow_runs": [{"head_sha": SHA, "head_branch": "main", "event": "push",
                                           "created_at": "2099", "id": 1, "status": "completed", "conclusion": "failure"}]}
            return {"workflow_runs": [run()]}
        with mock.patch.object(release, "gh", github), self.assertRaises(ValueError):
            release.validate_preconditions("ajent-social/ajent-social", SHA, CONTROL_SHA)


    def test_control_repository_must_be_private_and_fork_disabled(self):
        for private, forking in ((False, False), (True, True), (True, None)):
            with self.subTest(private=private, forking=forking):
                def github(endpoint):
                    if endpoint == "repos/ajent-social/ajent-social/commits/main":
                        return {"sha": SHA}
                    if endpoint == "repos/" + release.CONTROL_REPO:
                        return {"full_name": release.CONTROL_REPO, "private": private, "allow_forking": forking}
                    return {}
                with mock.patch.object(release, "gh", github), mock.patch.object(release, "workflow_runs", return_value=[
                        {"id": 1, "head_sha": SHA, "head_branch": "main", "event": "push", "status": "completed", "conclusion": "success"}]), self.assertRaises(ValueError):
                    release.validate_preconditions("ajent-social/ajent-social", SHA, CONTROL_SHA)

    def test_application_and_control_revisions_are_independent(self):
        def github(endpoint):
            if endpoint == "repos/ajent-social/ajent-social/commits/main":
                return {"sha": SHA}
            if endpoint == "repos/" + release.CONTROL_REPO:
                return {"full_name": release.CONTROL_REPO, "private": True, "allow_forking": False}
            if endpoint == f"repos/{release.CONTROL_REPO}/commits/main":
                return {"sha": CONTROL_SHA}
            return {"workflow_runs": []}
        def workflows(repo, workflow):
            if workflow == "test.yml":
                return [{"id": 1, "head_sha": SHA, "head_branch": "main", "event": "push", "status": "completed", "conclusion": "success"}]
            return []
        with mock.patch.object(release, "gh", github), mock.patch.object(release, "workflow_runs", workflows):
            self.assertEqual(release.validate_preconditions("ajent-social/ajent-social", SHA, CONTROL_SHA), set())
            with self.assertRaises(ValueError):
                release.validate_preconditions("ajent-social/ajent-social", SHA, SHA)
            with self.assertRaises(ValueError):
                release.validate_preconditions("ajent-social/ajent-social", CONTROL_SHA, CONTROL_SHA)

    def test_deleted_legacy_workflow_is_checked_through_repository_runs(self):
        endpoints = []
        def github(endpoint):
            endpoints.append(endpoint)
            if endpoint == "repos/ajent-social/ajent-social/commits/main":
                return {"sha": SHA}
            if endpoint == "repos/" + release.CONTROL_REPO:
                return {"full_name": release.CONTROL_REPO, "private": True, "allow_forking": False}
            if endpoint == f"repos/{release.CONTROL_REPO}/commits/main":
                return {"sha": CONTROL_SHA}
            if "repos/ajent-social/ajent-social/actions/runs?status=queued" in endpoint:
                return {"total_count": 1, "workflow_runs": [{"path": ".github/workflows/release.yml", "status": "queued"}]}
            return {"workflow_runs": []}
        def workflows(repo, workflow):
            if workflow == "test.yml":
                return [{"id": 1, "head_sha": SHA, "head_branch": "main", "event": "push", "status": "completed", "conclusion": "success"}]
            return []
        with mock.patch.object(release, "gh", github), mock.patch.object(release, "workflow_runs", workflows), self.assertRaises(ValueError):
            release.validate_preconditions("ajent-social/ajent-social", SHA, CONTROL_SHA)
        self.assertFalse(any("repos/ajent-social/ajent-social/actions/workflows/release.yml" in e for e in endpoints))

    def test_public_repository_run_cannot_match_control_dispatch(self):
        public_run = run()
        public_run["repository"]["full_name"] = "ajent-social/ajent-social"
        self.assertIsNone(release.matching_dispatch([public_run], set(), CONTROL_SHA, "2026-01-01T00:00:00Z"))
        with self.assertRaises(ValueError):
            release.verify_run(public_run, CONTROL_SHA)

class LaunchTests(unittest.TestCase):
    def setup_case(self, directory):
        args = argparse.Namespace(stack=STACK, receipt=str(Path(directory) / "receipt.json"), revision=SHA, control_revision=CONTROL_SHA,
                                  repo="ajent-social/ajent-social", timeout_seconds=60,
                                  executor_directory=Path(directory))
        effects = []
        secrets = []
        def github(endpoint, **kwargs):
            effects.append(("gh", endpoint, kwargs))
            if "generate-jitconfig" in endpoint:
                return {"runner": {"id": 99, "labels": [{"name": "self-hosted"}, {"name": "ajent-release-11"}]},
                        "encoded_jit_config": "secret-registration"}
            if endpoint.endswith("/11"):
                return run("completed", "success")
            return {}
        def aws(*args):
            effects.append(("aws", args))
            if "describe-task-definition" in args:
                return {"taskDefinition": {"taskDefinitionArn": OUTPUTS["taskDefinitionArn"],
                        "networkMode": "awsvpc", "requiresCompatibilities": ["FARGATE"],
                        "containerDefinitions": [{"name": "runner", "image":
                            "394824061124.dkr.ecr.us-west-2.amazonaws.com/ajent-release-runner@sha256:" + "b" * 64,
                            "secrets": [{"name": "AJENT_JIT_CONFIG", "valueFrom":
                            "arn:aws:ssm:us-west-2:394824061124:parameter/ajent/release-executor/jit"}]}]}}
            if "run-task" in args:
                return {"tasks": [{"taskArn": TASK}]}
            if "describe-tasks" in args:
                return {"tasks": [{"taskArn": TASK, "lastStatus": "STOPPED",
                                   "containers": [{"name": "runner", "exitCode": 0}]}]}
            return {}
        kwargs = {"gh_call": github, "aws_call": aws, "command_call": lambda args: json.dumps(OUTPUTS),
                  "set_jit_call": lambda stack, directory, value: secrets.append(value),
                  "preconditions": lambda repo, sha, control_sha: {1}, "runs_call": lambda *args: [run()],
                  "monotonic": lambda: 0, "sleep": lambda _: None}
        return args, kwargs, effects, secrets

    def test_success_is_exact_run_plus_clean_task_and_disables_jit(self):
        with tempfile.TemporaryDirectory() as directory:
            args, kwargs, effects, secrets = self.setup_case(directory)
            result = release.launch(args, **kwargs)
            self.assertEqual(result["status"], "WORKFLOW_SUCCEEDED")
            self.assertEqual(result["revision"], SHA)
            self.assertEqual(result["control_revision"], CONTROL_SHA)
            dispatches = [e for e in effects if e[0] == "gh" and e[1].endswith("/dispatches")]
            self.assertEqual(len(dispatches), 1)
            self.assertIn(release.CONTROL_REPO, dispatches[0][1])
            self.assertEqual(dispatches[0][2]["body"]["inputs"]["source_revision"], SHA)
            registrations = [e for e in effects if e[0] == "gh" and "generate-jitconfig" in e[1]]
            self.assertTrue(all(release.CONTROL_REPO in e[1] for e in registrations))
            self.assertEqual(secrets, ["secret-registration", "disabled"])
            self.assertEqual(os.stat(args.receipt).st_mode & 0o777, 0o600)
            self.assertNotIn("secret-registration", Path(args.receipt).read_text())
            self.assertEqual(sum("run-task" in e[1] for e in effects if e[0] == "aws"), 1)

    def test_existing_receipt_refuses_all_effects(self):
        with tempfile.TemporaryDirectory() as directory:
            args, kwargs, effects, secrets = self.setup_case(directory)
            Path(args.receipt).write_text("{}")
            with self.assertRaises(FileExistsError):
                release.launch(args, **kwargs)
            self.assertEqual(effects, [])
            self.assertEqual(secrets, [])

    def test_unknown_ecs_start_is_not_retried_and_jit_is_disabled(self):
        with tempfile.TemporaryDirectory() as directory:
            args, kwargs, effects, secrets = self.setup_case(directory)
            original = kwargs["aws_call"]
            def aws(*values):
                if "run-task" in values:
                    effects.append(("aws", values))
                    raise TimeoutError("uncertain response")
                return original(*values)
            kwargs["aws_call"] = aws
            with self.assertRaises(RuntimeError):
                release.launch(args, **kwargs)
            receipt = json.loads(Path(args.receipt).read_text())
            self.assertEqual(receipt["status"], "NEEDS_RECONCILIATION")
            self.assertIsNone(receipt["task_arn"])
            self.assertEqual(secrets[-1], "disabled")
            self.assertFalse(any("stop-task" in e[1] for e in effects if e[0] == "aws"))
            self.assertEqual(sum("run-task" in e[1] for e in effects if e[0] == "aws"), 1)

    def test_extra_platform_labels_prevent_task_launch(self):
        with tempfile.TemporaryDirectory() as directory:
            args, kwargs, effects, secrets = self.setup_case(directory)
            original = kwargs["gh_call"]
            def github(endpoint, **params):
                result = original(endpoint, **params)
                if "generate-jitconfig" in endpoint:
                    result["runner"]["labels"].append({"name": "Linux"})
                return result
            kwargs["gh_call"] = github
            with self.assertRaises(RuntimeError):
                release.launch(args, **kwargs)
            self.assertFalse(any("run-task" in e[1] for e in effects if e[0] == "aws"))
            self.assertEqual(secrets, [])

    def test_task_role_or_mutable_image_refuses_dispatch(self):
        for mutation in ("task_role", "mutable_image"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as directory:
                args, kwargs, effects, secrets = self.setup_case(directory)
                original = kwargs["aws_call"]
                def aws(*values):
                    result = original(*values)
                    if "describe-task-definition" in values:
                        if mutation == "task_role":
                            result["taskDefinition"]["taskRoleArn"] = "arn:aws:iam::394824061124:role/unapproved"
                        else:
                            result["taskDefinition"]["containerDefinitions"][0]["image"] = "repo:latest"
                    return result
                kwargs["aws_call"] = aws
                with self.assertRaises(RuntimeError):
                    release.launch(args, **kwargs)
                self.assertFalse(any(e[0] == "gh" for e in effects))
                self.assertEqual(secrets, [])

    def test_cleanup_failure_prevents_success(self):
        with tempfile.TemporaryDirectory() as directory:
            args, kwargs, effects, secrets = self.setup_case(directory)
            def set_secret(stack, directory, value):
                if value == "disabled":
                    raise RuntimeError("apply failed")
            kwargs["set_jit_call"] = set_secret
            with self.assertRaises(RuntimeError):
                release.launch(args, **kwargs)
            receipt = json.loads(Path(args.receipt).read_text())
            self.assertEqual(receipt["jit_cleanup"], "NEEDS_RECONCILIATION")
            self.assertEqual(receipt["status"], "NEEDS_RECONCILIATION")

    def test_partial_jit_apply_is_cleaned_up(self):
        with tempfile.TemporaryDirectory() as directory:
            args, kwargs, effects, secrets = self.setup_case(directory)
            def set_secret(stack, directory, value):
                secrets.append(value)
                if value != "disabled":
                    raise TimeoutError("unknown apply")
            kwargs["set_jit_call"] = set_secret
            with self.assertRaises(RuntimeError):
                release.launch(args, **kwargs)
            self.assertEqual(secrets, ["secret-registration", "disabled"])
            self.assertFalse(any("run-task" in e[1] for e in effects if e[0] == "aws"))


if __name__ == "__main__":
    unittest.main()
