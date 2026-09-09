"""Shared-skill installation must preserve links, settings and rollback boundaries."""
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


client = load('installation_path_client', 'join.py')
integrations = load('installation_path_integrations', 'ajent_integrations.py')


class InstallationPathTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.home = Path(self.temp.name).resolve() / 'home'
        self.home.mkdir(mode=0o700)
        self.directory = self.home / '.aj'
        self.directory.mkdir(mode=0o700)
        self.client_file = self.directory / 'client.py'
        self.client_file.write_text('existing client')
        self.skill_root = self.home / '.claude/skills'
        self.skill_root.mkdir(parents=True, mode=0o700)
        (self.home / '.agents').mkdir(mode=0o700)
        self.link = self.home / '.agents/skills'
        self.link.symlink_to(self.skill_root, target_is_directory=True)
        self.skill = self.skill_root / 'ajent-network/SKILL.md'
        for manager in [
            patch.object(Path, 'home', return_value=self.home),
            patch.dict(os.environ, {'AJENT_CONFIG_DIR': str(self.directory), 'AJENT_INTEGRATIONS': '1'}),
            patch.object(client, 'ORIGIN', 'https://ajent.test'),
            patch.object(integrations.shutil, 'which', return_value=None),
            patch.dict(client.sys.modules, {'ajent_integrations': integrations}),
            patch.object(client.sys, 'path', list(client.sys.path)),
        ]:
            manager.start()
            self.addCleanup(manager.stop)

    def install(self):
        files = {'marketplace/plugins/ajent/skills/network/SKILL.md': 'package skill'}
        with patch.object(client.urllib.request, 'build_opener') as opener:
            opener.return_value.open.return_value = io.BytesIO(json.dumps(files).encode())
            client.install_components(self.directory, self.client_file)

    def test_symlinked_shared_directory_installs_and_rolls_back(self):
        self.install()
        self.assertEqual(self.skill.read_text(), 'package skill')
        self.assertTrue(self.link.is_symlink())
        backup = next((self.directory / 'backups').iterdir())
        manifest = json.loads((backup / 'manifest.json').read_text())
        self.assertIn(str(self.skill), [entry['target'] for entry in manifest])
        # Retargeting the original link must not redirect rollback.
        other = self.home / 'other/ajent-network'
        other.mkdir(parents=True)
        (other / 'SKILL.md').write_text('unrelated skill')
        self.link.unlink()
        self.link.symlink_to(other.parent, target_is_directory=True)
        client.rollback_install(self.directory, backup.name)
        self.assertFalse(self.skill.exists())
        self.assertEqual((other / 'SKILL.md').read_text(), 'unrelated skill')
        self.assertTrue(self.link.is_symlink())

    def test_retargeted_discovery_link_cannot_redirect_install(self):
        other = self.home / 'other'
        other.mkdir()
        real_install = integrations.install
        def retarget(*args, **kwargs):
            self.link.unlink()
            self.link.symlink_to(other, target_is_directory=True)
            return real_install(*args, **kwargs)
        with patch.object(integrations, 'install', side_effect=retarget):
            self.install()
        self.assertEqual(self.skill.read_text(), 'package skill')
        self.assertEqual(list(other.iterdir()), [])

    def test_symlink_cycle_rejected(self):
        self.link.unlink()
        self.link.symlink_to(self.link, target_is_directory=True)
        with self.assertRaises(ValueError):
            self.install()
        self.assertFalse(self.skill.exists())

    def test_non_owned_target_rejected(self):
        original_stat = Path.stat
        def stat(path, *args, **kwargs):
            result = original_stat(path, *args, **kwargs)
            if path == self.skill_root:
                values = list(result)
                values[4] = os.getuid() + 1
                return os.stat_result(values)
            return result
        with patch.object(Path, 'stat', stat):
            with self.assertRaises(ValueError):
                self.install()
        self.assertFalse(self.skill.exists())

    def test_existing_independent_skill_is_preserved(self):
        self.skill.parent.mkdir()
        self.skill.write_text('independently managed skill')
        self.install()
        self.assertEqual(self.skill.read_text(), 'independently managed skill')
        self.assertTrue(self.link.is_symlink())

    def test_symlinked_skill_file_rejected_without_writing_target(self):
        self.skill.parent.mkdir()
        original = self.home / 'original.md'
        original.write_text('independent')
        self.skill.symlink_to(original)
        with self.assertRaises(ValueError):
            self.install()
        self.assertEqual(original.read_text(), 'independent')

    def test_symlink_outside_home_is_rejected(self):
        outside = self.home.parent / 'outside'
        outside.mkdir()
        self.link.unlink()
        self.link.symlink_to(outside, target_is_directory=True)
        with self.assertRaises(ValueError):
            self.install()
        self.assertEqual(list(outside.iterdir()), [])

    def test_shared_writable_target_is_rejected(self):
        self.skill_root.chmod(0o777)
        with self.assertRaises(ValueError):
            self.install()
        self.assertFalse(self.skill.exists())

    def test_config_symlink_remains_rejected(self):
        target = self.home / 'other-config.json'
        target.write_text('{}')
        (self.home / '.claude.json').symlink_to(target)
        with self.assertRaises(ValueError):
            self.install()
        self.assertEqual(target.read_text(), '{}')

    def test_client_only_install_does_not_inspect_or_change_skill(self):
        self.link.unlink()
        self.link.symlink_to(self.home.parent / 'missing', target_is_directory=True)
        with patch.dict(os.environ, {'AJENT_INTEGRATIONS': '0'}):
            self.install()
        self.assertTrue(self.link.is_symlink())
        self.assertFalse((self.home.parent / 'missing').exists())


if __name__ == '__main__':
    unittest.main()
