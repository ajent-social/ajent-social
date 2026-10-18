"""Run the release configuration restore step without cloud access."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class ReleaseConfigTests(unittest.TestCase):
    def restore_step(self):
        workflow = (ROOT / '.github/workflows/release.yml').read_text()
        title = '      - name: Restore deployed stack configuration\n'
        self.assertIn(title, workflow)
        block = workflow.split(title, 1)[1].split('\n      - ', 1)[0]
        return '\n'.join(line[10:] for line in block.split('        run: |\n', 1)[1].splitlines())

    def run_restore(self, fail=False):
        step = self.restore_step()
        with tempfile.TemporaryDirectory() as directory:
            tmp = Path(directory)
            (tmp / 'infra').mkdir()
            binary = tmp / 'pulumi'
            binary.write_text('#!/bin/sh\nif [ "$FAIL_RESTORE" = 1 ]; then exit 7; fi\n'
                              'printf "%s\\n" "$@" > args\n'
                              'printf "config: restored\\n" > infra/Pulumi.production.yaml\n')
            binary.chmod(0o700)
            env = dict(os.environ, PATH=str(tmp)+os.pathsep+os.environ['PATH'],
                       PULUMI_STACK='owner/ajent-infra/production', FAIL_RESTORE=str(int(fail)))
            result = subprocess.run(['bash', '-c', step], cwd=tmp, env=env,
                                    capture_output=True, text=True, timeout=10)
            config = tmp / 'infra/Pulumi.production.yaml'
            return result, config.exists(), config.stat().st_mode & 0o777 if config.exists() else None, (tmp/'args').read_text() if (tmp/'args').exists() else ''

    def test_restores_remote_config_with_private_file_mode(self):
        result, exists, mode, args = self.run_restore()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(exists)
        self.assertEqual(mode, 0o600)
        self.assertIn('owner/ajent-infra/production', args)
        self.assertIn('refresh', args)
        self.assertIn('--non-interactive', args)

    def test_failed_restore_stops_release_step(self):
        result, exists, _, _ = self.run_restore(fail=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(exists)
