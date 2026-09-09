"""Exercise the release script with synthetic commands, never cloud access."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = Path(os.environ.get('AJENT_DEPLOY_SCRIPT', ROOT / 'scripts/deploy.sh'))


class DeploymentGateTests(unittest.TestCase):
    def run_deploy(self, mode, verified=False, changed=False, fail_apply=False):
        with tempfile.TemporaryDirectory() as directory:
            tmp = Path(directory)
            commands = tmp / 'bin'
            commands.mkdir()
            log = tmp / 'calls.jsonl'
            receipt = {'changeSummary': {'same': 2}, 'steps': [
                {'op': 'same', 'urn': name, 'newState': {'type': 'test:resource:Resource'}}
                for name in ['one', 'two']
            ]}
            baseline = tmp / 'baseline.json'
            baseline.write_text(json.dumps(receipt))
            # Run the actual receipt comparator; do not recursively run this suite.
            stub = '#!' + sys.executable + '''
import json,os,sys
from pathlib import Path
name=Path(sys.argv[0]).name
args=sys.argv[1:]
with open(os.environ['MOCK_LOG'],'a') as f: f.write(json.dumps([name]+args)+'\\n')
if name=='python3':
 if args[:2]==['-m','unittest']: sys.exit(0)
 os.execv(sys.executable,[sys.executable]+args)
if name=='git' and args==['rev-parse','HEAD']: print('a'*40)
if name=='pulumi':
 if 'up' in args and os.environ['FAIL_APPLY']=='1': sys.exit(1)
 if 'get' in args:
  key=args[args.index('get')+1]
  if key=='goMigrationRevision':
   if os.environ['VERIFIED']=='1': print('a'*40)
   else: sys.exit(1)
  elif key=='aws:region': print('us-west-2')
 elif 'output' in args: print('https://example.test' if 'serviceUrl' in args else 'fleet-test')
 elif 'preview' in args and '--json' in args:
  receipt=json.loads(Path(os.environ['BASELINE']).read_text())
  if os.environ['CHANGED']=='1': receipt['changeSummary']['update']=1
  print(json.dumps(receipt))
if name=='aws': print('Successful' if any('Status' in a for a in args) else 'refresh-test')
'''
            for name in ['git', 'pulumi', 'aws', 'go', 'curl', 'python3']:
                path = commands / name
                path.write_text(stub)
                path.chmod(0o755)
            env = dict(os.environ, CI='true', PATH=str(commands)+os.pathsep+os.environ['PATH'],
                       MOCK_LOG=str(log), BASELINE=str(baseline), VERIFIED=str(int(verified)),
                       CHANGED=str(int(changed)), FAIL_APPLY=str(int(fail_apply)), TMPDIR=str(tmp))
            args = ['--migrate', str(baseline)] if mode == 'migration' else ['sha256:'+'b'*64]
            result = subprocess.run(['bash', str(SCRIPT), 'test-stack', *args], cwd=ROOT,
                                    env=env, capture_output=True, text=True, timeout=20)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_unverified_image_release_is_rejected_before_mutation(self):
        result, calls = self.run_deploy('image')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('set' in call or 'up' in call for call in calls))

    def test_migration_applies_saved_plan_before_marking_revision(self):
        result, calls = self.run_deploy('migration')
        self.assertEqual(result.returncode, 0, result.stderr)
        apply = next(i for i,c in enumerate(calls) if c[0]=='pulumi' and 'up' in c)
        marker = next(i for i,c in enumerate(calls) if 'set' in c and 'goMigrationRevision' in c)
        self.assertLess(apply, marker)
        self.assertIn('--plan', calls[apply])
        self.assertIn('--expect-no-changes', calls[apply])
        self.assertFalse(any('set' in c and 'imageDigest' in c for c in calls))

    def test_changed_migration_never_applies_or_marks_success(self):
        result, calls = self.run_deploy('migration', changed=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('up' in c or 'set' in c for c in calls))

    def test_failed_apply_never_marks_success(self):
        result, calls = self.run_deploy('migration', fail_apply=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('set' in c for c in calls))

    def test_verified_image_release_is_allowed(self):
        result, calls = self.run_deploy('image', verified=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(any('set' in c and 'imageDigest' in c for c in calls))
