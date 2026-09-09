import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch
s=importlib.util.spec_from_file_location('installer',Path(__file__).with_name('install-improvement-coordinator.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class InstallerTest(unittest.TestCase):
 def test_scheduler_uses_resolved_github_cli_location(self):
  with patch.object(m.shutil,'which',return_value='/custom/bin/gh'):
   c=m.configuration(Path('/run/coordinator.py'),Path('/private/credential.json'),'example/repo',Path('/private/state'),900)
  self.assertTrue(c['EnvironmentVariables']['PATH'].startswith('/custom/bin:'))
  self.assertEqual(c['StartInterval'],900);self.assertIn('--apply',c['ProgramArguments'])
