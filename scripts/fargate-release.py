#!/usr/bin/env python3
"""Launch and supervise one exact-main release on a one-job Fargate runner.

This owner-side helper makes no automatic retries of dispatch, registration,
Pulumi apply, or ECS launch. A private receipt is the recovery journal.
"""
import argparse
import copy
import datetime
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time
import uuid

REGION = "us-west-2"
CONTROL_REPO = "ajent-social/production-release"


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def save_receipt(path, receipt):
    with tempfile.NamedTemporaryFile(mode="w", dir=path.parent, delete=False) as stream:
        json.dump(receipt, stream, indent=2)
        stream.write("\n")
        temporary = stream.name
    os.chmod(temporary, 0o600)
    os.replace(temporary, path)


def command(args, *, body=None, env=None):
    """Never echo command output: registration responses can contain credentials."""
    environment = env
    if args and args[0] == "pulumi":
        # Only the owning Pulumi GitHub provider needs this token. Keep it in
        # the subprocess environment, never arguments, receipts or other jobs.
        environment = dict(os.environ if env is None else env)
        if not environment.get("GITHUB_TOKEN"):
            token = environment.get("GH_TOKEN")
            if not token:
                auth = subprocess.run(["gh", "auth", "token"], capture_output=True,
                                      text=True, check=True, timeout=30, env=environment)
                token = auth.stdout.strip()
            if not token:
                raise ValueError("GitHub provider authentication is unavailable")
            environment["GITHUB_TOKEN"] = token
    result = subprocess.run(args, input=body, capture_output=True, text=True,
                            check=True, timeout=180, env=environment)
    return result.stdout


def gh(endpoint, *, method="GET", body=None):
    args = ["gh", "api", "--method", method, endpoint]
    if body is not None:
        args += ["--input", "-"]
    output = command(args, body=json.dumps(body) if body is not None else None)
    return json.loads(output) if output.strip() else {}


def aws(*args):
    output = command(["aws", "--region", REGION, "--cli-connect-timeout", "10",
                      "--cli-read-timeout", "60", *args, "--output", "json"])
    return json.loads(output) if output.strip() else {}


def validate_jit_preview(output, stack):
    """Only a value update to this stack's existing JIT parameter is permitted."""
    expected = f"urn:pulumi:{stack.rsplit('/', 1)[-1]}::ajent-release-executor::aws:ssm/parameter:Parameter::jitParameter"
    changes = []
    summaries = []
    for line in output.splitlines():
        if not line.strip():
            continue
        event = json.loads(line)
        if event.get("diagnosticEvent", {}).get("severity") == "error":
            raise ValueError("JIT preview contains an error")
        if "summaryEvent" in event:
            summaries.append(event["summaryEvent"])
        meta = event.get("resourcePreEvent", {}).get("metadata")
        if not meta or meta.get("op") in ("same", "read", "read-discard"):
            continue
        if meta.get("type") == "pulumi:pulumi:Stack" and meta.get("op") == "update":
            continue
        if meta.get("op") != "update" or meta.get("urn") != expected or meta.get("type") != "aws:ssm/parameter:Parameter":
            raise ValueError("JIT preview changes an unexpected resource")
        old = copy.deepcopy(meta.get("old", {}).get("inputs", {}))
        new = copy.deepcopy(meta.get("new", {}).get("inputs", {}))
        if not old or not new or "value" not in old or "value" not in new:
            raise ValueError("JIT preview is missing parameter inputs")
        old.pop("value")
        new.pop("value")
        if old != new:
            raise ValueError("JIT preview changes parameter properties other than value")
        changes.append(meta)
    if len(summaries) != 1 or summaries[0].get("maybeCorrupt") or len(changes) > 1:
        raise ValueError("JIT preview is incomplete or ambiguous")
    summary = summaries[0].get("resourceChanges", {})
    if any(count for op, count in summary.items() if op not in ("same", "read", "read-discard", "update")):
        raise ValueError("JIT preview summary includes unexpected operations")
    if summary.get("update", 0) > len(changes) + 1:
        raise ValueError("JIT preview summary includes unobserved updates")
    return len(changes)


def set_jit(stack, directory, value):
    """Value travels on stdin, encrypted in config/state, never in argv or logs."""
    prefix = ["pulumi", "-C", str(directory)]
    command(prefix + ["config", "set", "jitConfig", "--secret", "--stack", stack,
                      "--non-interactive"], body=value + "\n")
    environment = dict(os.environ, PULUMI_EXPERIMENTAL="true",
                       PULUMI_ENABLE_STREAMING_JSON_PREVIEW="true")
    with tempfile.TemporaryDirectory(prefix="ajent-jit-plan-") as temporary:
        plan = str(Path(temporary) / "plan.json")
        output = command(prefix + ["preview", "--stack", stack, "--refresh=false", "--json",
                                   "--save-plan", plan, "--non-interactive"], env=environment)
        changes = validate_jit_preview(output, stack)
        # Apply a saved plan even for a no-op: the encrypted config and state
        # must agree before launch or cleanup can be recorded as complete.
        command(prefix + ["up", "--stack", stack, "--refresh=false", "--plan", plan,
                          "--yes", "--non-interactive"], env=environment)
        return changes


def validate_executor(outputs, aws_call):
    expected_cluster = "arn:aws:ecs:us-west-2:394824061124:cluster/ajent-release"
    definition_arn = outputs.get("taskDefinitionArn", "")
    if outputs.get("clusterArn") != expected_cluster or not re.fullmatch(
            r"arn:aws:ecs:us-west-2:394824061124:task-definition/ajent-release-runner:[0-9]+", definition_arn):
        raise ValueError("Unexpected executor identity")
    result = aws_call("ecs", "describe-task-definition", "--task-definition", definition_arn)
    definition = result.get("taskDefinition", {})
    if definition.get("taskDefinitionArn") != definition_arn or definition.get("taskRoleArn"):
        raise ValueError("Runner must have no IAM task role")
    containers = definition.get("containerDefinitions", [])
    if len(containers) != 1 or containers[0].get("name") != "runner":
        raise ValueError("Executor must contain exactly the reviewed runner")
    container = containers[0]
    if not re.fullmatch(r"394824061124\.dkr\.ecr\.us-west-2\.amazonaws\.com/ajent-release-runner@sha256:[a-f0-9]{64}", container.get("image", "")):
        raise ValueError("Executor runner image must use an immutable digest")
    expected_secret = [{"name": "AJENT_JIT_CONFIG", "valueFrom":
                        "arn:aws:ssm:us-west-2:394824061124:parameter/ajent/release-executor/jit"}]
    if container.get("secrets") != expected_secret:
        raise ValueError("Executor may receive only its one-use runner configuration")
    if definition.get("networkMode") != "awsvpc" or "FARGATE" not in definition.get("requiresCompatibilities", []):
        raise ValueError("Executor is not the isolated Fargate task")


def workflow_runs(repo, workflow):
    return gh(f"repos/{repo}/actions/workflows/{workflow}/runs?per_page=100").get("workflow_runs", [])


def validate_preconditions(repo, revision, control_revision):
    if gh(f"repos/{repo}/commits/main").get("sha") != revision:
        raise ValueError("Selected application revision must remain current main")
    tests = [run for run in workflow_runs(repo, "test.yml")
             if run.get("head_sha") == revision and run.get("head_branch") == "main" and run.get("event") == "push"]
    tests.sort(key=lambda run: (run.get("created_at", ""), run.get("id", 0)), reverse=True)
    if not tests or tests[0].get("status") != "completed" or tests[0].get("conclusion") != "success":
        raise ValueError("Latest exact-main application push CI must have succeeded")
    control = gh(f"repos/{CONTROL_REPO}")
    if control.get("full_name") != CONTROL_REPO or control.get("private") is not True or control.get("allow_forking") is not False:
        raise ValueError("Control repository must be private with forking disabled")
    if gh(f"repos/{CONTROL_REPO}/commits/main").get("sha") != control_revision:
        raise ValueError("Selected control revision must remain current main")
    # The legacy public workflow is removed during migration. Query repository
    # runs by path so a deleted workflow's filename endpoint is not required.
    for status in ("queued", "in_progress", "waiting", "requested", "pending"):
        active = gh(f"repos/{repo}/actions/runs?status={status}&per_page=100")
        runs = active.get("workflow_runs", [])
        if active.get("total_count", len(runs)) > len(runs):
            raise ValueError("Legacy release check is incomplete; reconcile active runs")
        if any(run.get("path", "").split("@", 1)[0] == ".github/workflows/release.yml" for run in runs):
            raise ValueError("A legacy application release is active; reconcile it first")
    releases = workflow_runs(CONTROL_REPO, "release.yml")
    if any(run.get("status") != "completed" for run in releases):
        raise ValueError("An existing control release is active; reconcile it first")
    # Old queued work must not disappear behind 100 newer completed runs.
    for status in ("queued", "in_progress", "waiting", "requested", "pending"):
        active = gh(f"repos/{CONTROL_REPO}/actions/workflows/release.yml/runs?status={status}&per_page=1")
        if active.get("workflow_runs"):
            raise ValueError("An existing control release is active; reconcile it first")
    return {run["id"] for run in releases}


def matching_dispatch(runs, baseline, revision, requested_at):
    matches = [run for run in runs if run.get("id") not in baseline and
               run.get("event") == "workflow_dispatch" and run.get("head_sha") == revision and
               run.get("head_branch") == "main" and run.get("repository", {}).get("full_name") == CONTROL_REPO and
               run.get("created_at", "") >= requested_at]
    if len(matches) > 1:
        raise ValueError("Dispatch result is ambiguous; do not start or cancel unidentified releases")
    return matches[0] if matches else None


def verify_run(run, revision):
    if (run.get("head_sha") != revision or run.get("event") != "workflow_dispatch" or
            run.get("head_branch") != "main" or run.get("repository", {}).get("full_name") != CONTROL_REPO):
        raise ValueError("Workflow run identity changed")


def launch(args, *, gh_call=gh, aws_call=aws, command_call=command, set_jit_call=set_jit,
           preconditions=validate_preconditions, runs_call=workflow_runs,
           monotonic=time.monotonic, sleep=time.sleep):
    if not re.fullmatch(r"[a-f0-9]{40}", args.revision):
        raise ValueError("--revision requires a full commit SHA")
    if not re.fullmatch(r"[a-f0-9]{40}", args.control_revision):
        raise ValueError("--control-revision requires the full reviewed control commit SHA")
    if args.repo != "ajent-social/ajent-social":
        raise ValueError("This executor is restricted to ajent-social/ajent-social")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/ajent-release-executor/[A-Za-z0-9_.-]+", args.stack):
        raise ValueError("Use the full owning ajent-release-executor stack name")
    if not 60 <= args.timeout_seconds <= 10800:
        raise ValueError("Timeout must be 60..10800 seconds")
    path = Path(args.receipt)
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    receipt = {"created_at": utc_now(), "revision": args.revision, "repo": args.repo,
               "control_revision": args.control_revision, "control_repo": CONTROL_REPO,
               "stack": args.stack, "status": "PREPARING", "phase": "preconditions",
               "run_id": None, "task_arn": None, "runner_id": None,
               "client_token": uuid.uuid4().hex, "jit_cleanup": "NOT_NEEDED"}
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(receipt, stream, indent=2)
    deadline = monotonic() + args.timeout_seconds
    jit_attempted = False
    outputs = None
    complete = False
    primary_error = None
    try:
        baseline = preconditions(args.repo, args.revision, args.control_revision)
        outputs = json.loads(command_call(["pulumi", "-C", str(args.executor_directory),
                                          "stack", "output", "--stack", args.stack, "--json"]))
        required = ("clusterArn", "taskDefinitionArn", "securityGroupId", "subnetId")
        if any(not isinstance(outputs.get(key), str) or not outputs[key] for key in required):
            raise ValueError("Executor outputs are incomplete")
        validate_executor(outputs, aws_call)
        receipt["cluster_arn"] = outputs["clusterArn"]
        receipt["task_definition_arn"] = outputs["taskDefinitionArn"]
        receipt["dispatch_requested_at"] = utc_now()
        receipt["baseline_run_ids"] = sorted(baseline)
        receipt["phase"] = "dispatch"
        receipt["status"] = "DISPATCH_REQUESTED"
        save_receipt(path, receipt)
        gh_call(f"repos/{CONTROL_REPO}/actions/workflows/release.yml/dispatches", method="POST",
                body={"ref": "main", "inputs": {"confirm_production": True, "source_revision": args.revision}})
        # The old dispatch endpoint returns no run ID. Correlate a single new
        # exact-head run; never resend the dispatch if its result is uncertain.
        correlation_deadline = min(deadline, monotonic() + 120)
        run = None
        while monotonic() < correlation_deadline:
            run = matching_dispatch(runs_call(CONTROL_REPO, "release.yml"), baseline,
                                    args.control_revision, receipt["dispatch_requested_at"])
            if run:
                break
            sleep(5)
        if run is None:
            raise TimeoutError("Dispatch outcome unknown; reconcile before another attempt")
        receipt["run_id"] = run["id"]
        receipt["run_url"] = run.get("html_url")
        receipt["phase"] = "jit_registration"
        save_receipt(path, receipt)
        verify_run(run, args.control_revision)
        if run.get("status") == "completed":
            raise RuntimeError("Release completed before runner launch")
        label = f"ajent-release-{run['id']}"
        registration = gh_call(f"repos/{CONTROL_REPO}/actions/runners/generate-jitconfig", method="POST",
                               body={"name": label, "runner_group_id": 1,
                                     "labels": ["self-hosted", label], "work_folder": "_work"})
        runner = registration.get("runner", {})
        runner_id = runner.get("id")
        if not isinstance(runner_id, int):
            raise ValueError("JIT registration did not return an exact runner ID")
        receipt["runner_id"] = runner_id
        save_receipt(path, receipt)
        labels = {entry.get("name") for entry in runner.get("labels", [])}
        if labels != {"self-hosted", label}:
            raise ValueError("JIT registration includes unexpected labels; PR-job routing is forbidden")
        jit = registration.get("encoded_jit_config")
        if not isinstance(jit, str) or not jit:
            raise ValueError("JIT registration response lacks configuration")
        receipt["phase"] = "jit_apply"
        receipt["jit_cleanup"] = "REQUIRED"
        save_receipt(path, receipt)
        jit_attempted = True
        set_jit_call(args.stack, args.executor_directory, jit)
        del jit, registration
        if monotonic() >= deadline:
            raise TimeoutError("Deadline elapsed before ECS launch")
        receipt["phase"] = "ecs_launch"
        receipt["status"] = "TASK_START_REQUESTED"
        save_receipt(path, receipt)
        network = {"awsvpcConfiguration": {"subnets": [outputs["subnetId"]],
                   "securityGroups": [outputs["securityGroupId"]], "assignPublicIp": "ENABLED"}}
        started = aws_call("ecs", "run-task", "--cluster", outputs["clusterArn"],
                           "--task-definition", outputs["taskDefinitionArn"], "--launch-type", "FARGATE",
                           "--count", "1", "--client-token", receipt["client_token"],
                           "--network-configuration", json.dumps(network))
        tasks = started.get("tasks", [])
        if len(tasks) != 1 or started.get("failures"):
            raise RuntimeError("ECS launch did not identify exactly one successful task")
        task_arn = tasks[0].get("taskArn")
        if not isinstance(task_arn, str) or not task_arn.startswith("arn:aws:ecs:us-west-2:394824061124:task/ajent-release/"):
            raise ValueError("Unexpected ECS task identity")
        receipt["task_arn"] = task_arn
        receipt["status"] = "RUNNING"
        receipt["phase"] = "supervision"
        save_receipt(path, receipt)
        print(f"Release {receipt['run_id']} and Fargate task started; receipt: {path}", file=sys.stderr)
        stopped_deadline = None
        while monotonic() < deadline:
            run = gh_call(f"repos/{CONTROL_REPO}/actions/runs/{receipt['run_id']}")
            verify_run(run, args.control_revision)
            result = aws_call("ecs", "describe-tasks", "--cluster", outputs["clusterArn"], "--tasks", task_arn)
            tasks = result.get("tasks", [])
            if len(tasks) != 1 or tasks[0].get("taskArn") != task_arn or result.get("failures"):
                raise RuntimeError("Known ECS task could not be reconciled")
            task = tasks[0]
            receipt["workflow_status"] = run.get("status")
            receipt["workflow_conclusion"] = run.get("conclusion")
            receipt["task_status"] = task.get("lastStatus")
            stopped = task.get("lastStatus") == "STOPPED"
            if stopped:
                containers = [c for c in task.get("containers", []) if c.get("name") == "runner"]
                receipt["runner_exit_code"] = containers[0].get("exitCode") if len(containers) == 1 else None
                receipt["task_stopped_reason"] = task.get("stopCode")
            save_receipt(path, receipt)
            if run.get("status") == "completed":
                if run.get("conclusion") != "success":
                    raise RuntimeError("Production workflow failed; preserve its exact evidence")
                if stopped:
                    if receipt.get("runner_exit_code") != 0:
                        raise RuntimeError("Runner task did not exit successfully")
                    complete = True
                    break
            elif stopped:
                if receipt.get("runner_exit_code") != 0:
                    raise RuntimeError("Runner stopped unsuccessfully before workflow completion")
                # GitHub completion can lag the runner's clean shutdown. Permit
                # bounded API convergence, never a second runner or dispatch.
                if stopped_deadline is None:
                    stopped_deadline = min(deadline, monotonic() + 60)
                if monotonic() >= stopped_deadline:
                    raise RuntimeError("Runner stopped but workflow completion remains unknown")
            sleep(15)
        if not complete:
            raise TimeoutError("Release supervision deadline exceeded")
        receipt["status"] = "WORKFLOW_SUCCEEDED"
    except BaseException as error:
        primary_error = error
        receipt["status"] = "NEEDS_RECONCILIATION"
        receipt["error_type"] = type(error).__name__
        # Cleanup targets only identities this attempt actually received.
        for key, effect in (
            ("run_id", lambda: gh_call(f"repos/{CONTROL_REPO}/actions/runs/{receipt['run_id']}/cancel", method="POST")),
            ("task_arn", lambda: aws_call("ecs", "stop-task", "--cluster", outputs["clusterArn"],
                                         "--task", receipt["task_arn"], "--reason", "Owner release supervisor stopped")),
            ("runner_id", lambda: gh_call(f"repos/{CONTROL_REPO}/actions/runners/{receipt['runner_id']}", method="DELETE")),
        ):
            if receipt.get(key):
                try:
                    effect()
                    receipt[key + "_cleanup_requested"] = True
                except BaseException as cleanup_error:
                    receipt[key + "_cleanup_error"] = type(cleanup_error).__name__
    finally:
        if jit_attempted:
            try:
                set_jit_call(args.stack, args.executor_directory, "disabled")
                receipt["jit_cleanup"] = "DISABLED_VERIFIED_APPLY"
            except BaseException as cleanup_error:
                receipt["jit_cleanup"] = "NEEDS_RECONCILIATION"
                receipt["jit_cleanup_error"] = type(cleanup_error).__name__
                receipt["status"] = "NEEDS_RECONCILIATION"
                if primary_error is None:
                    primary_error = RuntimeError("JIT credential cleanup requires reconciliation")
        receipt["finished_at"] = utc_now()
        save_receipt(path, receipt)
    if primary_error:
        raise RuntimeError(f"Release did not complete cleanly; see private receipt: {path}") from None
    return receipt


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stack", required=True)
    parser.add_argument("--receipt", required=True)
    parser.add_argument("--revision", required=True, help="Exact application main commit")
    parser.add_argument("--control-revision", required=True, help="Exact reviewed private control main commit")
    parser.add_argument("--repo", default="ajent-social/ajent-social")
    parser.add_argument("--timeout-seconds", type=int, default=10800)
    parser.add_argument("--executor-directory", type=Path,
                        default=Path(__file__).resolve().parents[1] / "infra" / "release-executor")
    return parser.parse_args(argv)


def main():
    def terminate(_signum, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, terminate)
    try:
        result = launch(parse_args())
        print(json.dumps({"status": result["status"], "run_id": result["run_id"],
                          "run_url": result.get("run_url"), "revision": result["revision"],
                          "control_revision": result["control_revision"]}))
    except (Exception, KeyboardInterrupt) as error:
        print(str(error) if isinstance(error, RuntimeError) else "Release stopped; inspect its private receipt.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
