"""Keep source-read credentials separate from control and deployment authority."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[3]
ACTION = "actions/create-github-app-token@64ab729a6294000f5aa4a4bc0b73d1f32bc45510"


class SourceAppBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.workflow = (ROOT / "runners/release/workflow.yml").read_text()
        self.program = json.loads((ROOT / "infra/release-executor/Pulumi.yaml").read_text())
        self.steps = re.split(r"^      - ", self.workflow, flags=re.MULTILINE)[1:]

    def test_both_tokens_are_pinned_to_one_repo_and_only_read_permissions(self):
        tokens = [step for step in self.steps if ACTION in step]
        self.assertEqual(len(tokens), 2)
        for step in tokens:
            with self.subTest(step=step.splitlines()[0]):
                self.assertIn("client-id: ${{ vars.AJENT_SOURCE_APP_CLIENT_ID }}", step)
                self.assertIn("private-key: ${{ secrets.AJENT_SOURCE_APP_PRIVATE_KEY }}", step)
                self.assertIn("owner: ajent-social\n", step)
                self.assertIn("repositories: ajent-social\n", step)
                self.assertEqual(set(re.findall(r"permission-([a-z-]+): ([^\n]+)", step)),
                                 {("contents", "read"), ("actions", "read")})
                # Default post-job revocation must stay enabled for both tokens.
                self.assertNotIn("skip-token-revoke:", step)
                self.assertNotIn("app-id:", step)

    def test_checkout_and_ci_lookup_use_scoped_token_without_fallback(self):
        checkouts = [step for step in self.steps if "uses: actions/checkout@v4" in step]
        self.assertEqual(len(checkouts), 2)
        self.assertIn("token: ${{ steps.source-token.outputs.token }}", checkouts[0])
        self.assertIn("token: ${{ steps.source-token-refresh.outputs.token }}", checkouts[1])
        for step in checkouts:
            self.assertIn("repository: ajent-social/ajent-social", step)
            self.assertIn("ref: ${{ inputs.source_revision }}", step)
            self.assertIn("fetch-depth: 0", step)
        ci = next(step for step in self.steps if step.startswith("name: Require successful main CI"))
        self.assertIn("GH_TOKEN: ${{ steps.source-token.outputs.token }}", ci)
        self.assertIn('--repo "ajent-social/ajent-social" --workflow test.yml --commit "$AJENT_SOURCE_REVISION"', ci)
        self.assertIn('.event == "push" and .headSha == env.AJENT_SOURCE_REVISION', ci)
        self.assertIn('== "completed"', ci)
        self.assertIn('== "success"', ci)
        self.assertNotIn("${{ github.token }}", self.workflow)

    def test_missing_app_inputs_fail_before_initial_checkout_without_printing_secrets(self):
        validation = next(step for step in self.steps if step.startswith("name: Validate release configuration"))
        self.assertIn("AJENT_SOURCE_APP_CLIENT_ID: ${{ vars.AJENT_SOURCE_APP_CLIENT_ID }}", validation)
        self.assertIn("AJENT_SOURCE_APP_PRIVATE_KEY: ${{ secrets.AJENT_SOURCE_APP_PRIVATE_KEY }}", validation)
        shell = validation.split("run: |", 1)[1]
        self.assertIn('[[ -n "$AJENT_SOURCE_APP_CLIENT_ID" && "$AJENT_SOURCE_APP_CLIENT_ID" != disabled ]]', shell)
        self.assertIn('[[ -n "$AJENT_SOURCE_APP_PRIVATE_KEY" ]]', shell)
        self.assertNotIn("${{ vars.AJENT_SOURCE_APP_CLIENT_ID }}", shell)
        self.assertNotIn("${{ secrets.AJENT_SOURCE_APP_PRIVATE_KEY }}", shell)
        self.assertNotRegex(shell, r"(?:echo|printf)[^\n]*\$AJENT_SOURCE_APP_(?:PRIVATE_KEY|CLIENT_ID)")
        self.assertLess(self.workflow.index("name: Validate release configuration"), self.workflow.index("id: source-token"))
        self.assertLess(self.workflow.index("id: source-token"), self.workflow.index("uses: actions/checkout@v4"))

    def test_deployment_refresh_retains_restored_encrypted_config_and_exact_sha(self):
        checkout = next(step for step in self.steps if step.startswith("name: Refresh exact source checkout credentials"))
        self.assertIn("clean: false", checkout)
        self.assertIn("persist-credentials: true", checkout)
        self.assertIn("ref: ${{ inputs.source_revision }}", checkout)
        self.assertIn("fetch-depth: 0", checkout)
        self.assertLess(self.workflow.index("name: Publish tested ARM64 image with CodeBuild"),
                        self.workflow.index("id: source-token-refresh"))
        self.assertLess(self.workflow.index("id: source-token-refresh"),
                        self.workflow.index("name: Refresh exact source checkout credentials"))
        self.assertLess(self.workflow.index("name: Refresh exact source checkout credentials"),
                        self.workflow.index("name: Apply release and wait for rollout"))
        # Main must be fetched and compared before initial work; deploy.sh also
        # fetches main using the fresh persisted checkout credentials.
        self.assertIn("git fetch origin main", self.workflow)
        self.assertIn('[[ "$(git rev-parse origin/main)" == "$AJENT_SOURCE_REVISION" ]]', self.workflow)
        deploy = (ROOT / "scripts/deploy.sh").read_text()
        self.assertIn("git fetch origin main", deploy)
        self.assertIn("git merge-base --is-ancestor origin/main HEAD", deploy)

    def test_iac_manages_only_public_client_id_not_private_key(self):
        self.assertEqual(self.program["config"]["sourceAppClientId"], {"type": "string", "default": "disabled"})
        variable = self.program["resources"]["variableAJENT_SOURCE_APP_CLIENT_ID"]
        self.assertEqual(variable["type"], "github:index/actionsVariable:ActionsVariable")
        self.assertEqual(variable["properties"], {"repository": "${releaseRepository.name}",
                                                 "variableName": "AJENT_SOURCE_APP_CLIENT_ID",
                                                 "value": "${sourceAppClientId}"})
        self.assertEqual(variable["options"]["provider"], "${githubProvider}")
        serialized = json.dumps(self.program)
        self.assertNotIn("AJENT_SOURCE_APP_PRIVATE_KEY", serialized)
        self.assertNotIn("sourceAppPrivateKey", serialized)
        self.assertEqual(serialized.count("${sourceAppClientId}"), 1)
        secrets = [name for name, resource in self.program["resources"].items()
                   if resource["type"] == "github:index/actionsSecret:ActionsSecret"]
        self.assertEqual(secrets, ["pulumiToken"])
        for resource in self.program["resources"].values():
            if resource["type"].startswith("aws:"):
                self.assertNotIn("sourceAppClientId", json.dumps(resource))


if __name__ == "__main__":
    unittest.main()
