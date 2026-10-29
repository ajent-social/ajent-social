"""Checkpoint fixtures and release-step execution with a local Pulumi stub."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
HELPER = ROOT / 'scripts/deployed-host-ami.py'
spec = importlib.util.spec_from_file_location('host_ami', HELPER)
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)
AMI = 'ami-0123456789abcdef0'


def checkpoint():
    return {'deployment': {'resources': [{
        'type': 'aws:ec2/launchTemplate:LaunchTemplate',
        'urn': 'urn:pulumi:production::ajent-infra::aws:ec2/launchTemplate:LaunchTemplate::application-fleet',
        'inputs': {'imageId': AMI}, 'outputs': {'imageId': 'ami-fffffffffffffffff'},
    }]}}


class DeployedHostAMITests(unittest.TestCase):
    def test_uses_exact_inputs_and_ignores_unrelated_resources(self):
        data = checkpoint()
        other = copy.deepcopy(data['deployment']['resources'][0])
        other['urn'] += '-other'
        other['inputs']['imageId'] = 'invalid'
        data['deployment']['resources'].append(other)
        self.assertEqual(helper.deployed_host_ami(data), AMI)

    def test_rejects_missing_ambiguous_or_malformed_template(self):
        base = checkpoint()
        template = base['deployment']['resources'][0]
        cases = [{}, {'deployment': {'resources': []}},
                 {'deployment': {'resources': [template, template]}},
                 {'deployment': {'resources': [None]}}]
        for field, value in [('type', 'other'), ('urn', template['urn']+'-other'),
                             ('inputs', {}), ('inputs', None), ('delete', True),
                             ('pendingReplacement', True)]:
            data = checkpoint()
            data['deployment']['resources'][0][field] = value
            cases.append(data)
        for value in [None, 42, {}, '', 'ami-test', 'ami-0123456789ABCDEF0',
                      'ami-0123456789abcdef', AMI+'\n', ' '+AMI]:
            data = checkpoint()
            data['deployment']['resources'][0]['inputs']['imageId'] = value
            cases.append(data)
        for data in cases:
            with self.subTest(data=data), self.assertRaises(ValueError):
                helper.deployed_host_ami(data)

    def test_valid_identifier_lengths(self):
        for value in ['ami-0123abcd', AMI]:
            self.assertTrue(helper.immutableAMI(value))

    def step(self):
        workflow = (ROOT / 'runners/release/workflow.yml').read_text()
        names = ['Restore deployed stack configuration', 'Require completed Go infrastructure migration',
                 'Preserve deployed host AMI for application release',
                 'Verify infrastructure program is unchanged before publishing', 'Publish tested ARM64 image']
        positions = [workflow.index('- name: '+name) for name in names]
        self.assertEqual(positions, sorted(positions))
        self.assertIn('--refresh=false --expect-no-changes', workflow)
        block = workflow.split('      - name: '+names[2]+'\n')[1].split('\n      - ', 1)[0]
        return '\n'.join(line[10:] for line in block.split('        run: |\n')[1].splitlines())

    def test_release_step_private_checkpoint_cleanup_and_fail_closed(self):
        for mode in ['valid', 'invalid', 'export-failure', 'config-failure']:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                tmp = Path(directory)
                (tmp / 'scripts').mkdir()
                (tmp / 'scripts/deployed-host-ami.py').write_text(HELPER.read_text())
                data = checkpoint()
                if mode == 'invalid':
                    data['deployment']['resources'] = []
                (tmp / 'fixture').write_text(json.dumps(data))
                stub = tmp / 'pulumi'
                stub.write_text('''#!/usr/bin/env python3
import os, pathlib, sys
args = sys.argv[1:]
if args[2:4] == ['stack', 'export']:
    target = pathlib.Path(args[args.index('--file')+1])
    pathlib.Path('checkpoint-path').write_text(str(target))
    target.write_text(pathlib.Path('fixture').read_text())
    pathlib.Path('checkpoint-mode').write_text(oct(target.stat().st_mode & 0o777))
    sys.exit(7 if os.environ['MODE'] == 'export-failure' else 0)
if args[2:5] == ['config', 'set', 'hostAmi']:
    pathlib.Path('config-args').write_text(' '.join(args))
    sys.exit(8 if os.environ['MODE'] == 'config-failure' else 0)
sys.exit(9)
''')
                stub.chmod(0o700)
                result = subprocess.run(['bash', '-c', self.step()], cwd=tmp,
                                        env=dict(os.environ, PATH=str(tmp)+os.pathsep+os.environ['PATH'],
                                                 MODE=mode, PULUMI_STACK='test/production'),
                                        capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode == 0, mode == 'valid', result.stderr)
                self.assertFalse(Path((tmp/'checkpoint-path').read_text()).exists())
                self.assertEqual((tmp/'checkpoint-mode').read_text(), '0o600')
                config = tmp/'config-args'
                self.assertEqual(config.exists(), mode in ['valid', 'config-failure'])
                if config.exists():
                    self.assertEqual(config.read_text(), '-C infra config set hostAmi '+AMI+' --stack test/production --non-interactive')
                self.assertNotIn('deployment', result.stdout+result.stderr)

    def test_cli_never_echoes_bad_checkpoint(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)/'checkpoint'
            source.write_text('{sensitive-checkpoint-content')
            result = subprocess.run(['python3', str(HELPER), str(source)], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, '')
            self.assertNotIn('sensitive-checkpoint-content', result.stderr)
