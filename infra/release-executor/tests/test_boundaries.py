"""Fail closed if the executor gains production authority or PR exposure."""
import json
from pathlib import Path
import unittest

PROGRAM = json.loads((Path(__file__).resolve().parents[1] / 'Pulumi.yaml').read_text())
RESOURCES = PROGRAM['resources']


class CredentialBoundaries(unittest.TestCase):
    def test_config_names_cannot_shadow_resources(self):
        self.assertFalse(set(PROGRAM['config']) & set(RESOURCES))
        self.assertEqual(RESOURCES['pulumiToken']['properties']['plaintextValue'], '${pulumiAccessToken}')

    def test_real_jit_configuration_fits_encrypted_parameter(self):
        parameter = RESOURCES['jitParameter']['properties']
        self.assertEqual(parameter['type'], 'SecureString')
        observed_jit_characters = 4124  # Size-only probe; no credential content.
        limits = {'Standard': 4096, 'Advanced': 8192}
        self.assertLessEqual(observed_jit_characters, limits[parameter.get('tier', 'Standard')])

    def test_task_has_no_aws_principal(self):
        task = RESOURCES['runnerTask']['properties']
        self.assertNotIn('taskRoleArn', task)
        self.assertEqual(task['executionRoleArn'], '${executionRole.arn}')
        containers = task['containerDefinitions']['fn::toJSON']
        self.assertEqual(containers[0]['secrets'], [{'name': 'AJENT_JIT_CONFIG', 'valueFrom': '${jitParameter.arn}'}])

    def test_oidc_cannot_accept_pr_or_other_repository(self):
        statements = json.loads(RESOURCES['imageBuildRole']['properties']['assumeRolePolicy'])['Statement']
        self.assertEqual(len(statements), 1)
        condition = statements[0]['Condition']['StringEquals']
        subjects = condition['token.actions.githubusercontent.com:sub']
        self.assertEqual(subjects, [
            'repo:ajent-social/production-release:ref:refs/heads/main',
            'repo:ajent-social@325803062/production-release@1437942819:ref:refs/heads/main',
        ])
        repo = RESOURCES['releaseRepository']['properties']
        self.assertEqual(repo['visibility'], 'private')
        self.assertNotIn('allowForking', repo)  # Organization enforces this; launcher verifies actual false.
        self.assertEqual(condition['token.actions.githubusercontent.com:aud'], 'sts.amazonaws.com')

    def test_no_builder_can_deploy_or_read_runtime_secrets(self):
        for name in ('applicationBuilderPolicy', 'runnerBuilderPolicy', 'imageBuildPolicy', 'executionPolicy'):
            for statement in json.loads(RESOURCES[name]['properties']['policy'])['Statement']:
                for action in statement['Action']:
                    self.assertNotEqual(action, '*')
                    self.assertNotIn(action.split(':')[0], ('iam', 'autoscaling', 'ec2', 'secretsmanager', 'kms'))
                if name != 'executionPolicy':
                    self.assertFalse(any(a.startswith('ssm:') for a in statement['Action']))

    def test_rollout_identity_uses_approved_existing_permissions(self):
        expected = json.loads((Path(__file__).resolve().parents[1] / 'deploy-policy.json').read_text())
        actual = json.loads(RESOURCES['deploymentPolicy']['properties']['policy'])
        self.assertEqual(actual, expected)
        actions = [a for entry in actual['Statement'] for a in entry['Action']]
        self.assertNotIn('secretsmanager:GetSecretValue', actions)
        self.assertNotIn('iam:UpdateAssumeRolePolicy', actions)

    def test_saved_plan_policy_dependencies_are_explicit(self):
        for name, required in {
            'executionPolicy': {'executionRole', 'runnerRepository', 'logs', 'jitParameter'},
            'applicationBuilderPolicy': {'applicationBuilderRole', 'sources', 'logs'},
            'runnerBuilderPolicy': {'runnerBuilderRole', 'sources', 'logs', 'runnerRepository'},
        }.items():
            self.assertEqual(set(RESOURCES[name]['options']['dependsOn']), {'${' + x + '}' for x in required})

    def test_sources_are_private_versioned_and_expiring(self):
        self.assertEqual(RESOURCES['sourceVersioning']['properties']['versioningConfiguration']['status'], 'Enabled')
        privacy = RESOURCES['sourcePrivacy']['properties']
        for name in ('blockPublicAcls', 'blockPublicPolicy', 'ignorePublicAcls', 'restrictPublicBuckets'):
            self.assertTrue(privacy[name])
        self.assertEqual(RESOURCES['sourceLifecycle']['properties']['rules'][0]['expiration']['days'], 7)

    def test_no_always_on_service_or_ingress(self):
        self.assertFalse(any(r['type'] in ('aws:ecs:Service', 'aws:ec2:NatGateway') for r in RESOURCES.values()))
        self.assertEqual(RESOURCES['runnerSecurityGroup']['properties']['ingress'], [])
        for name in ('runnerBuilder', 'applicationBuilder'):
            props = RESOURCES[name]['properties']
            self.assertEqual(props['concurrentBuildLimit'], 1)
            self.assertLessEqual(props['buildTimeout'], 60)


if __name__ == '__main__':
    unittest.main()
