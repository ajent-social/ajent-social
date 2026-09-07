import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('join_client', Path(__file__).with_name('join.py'))
client = importlib.util.module_from_spec(spec)
spec.loader.exec_module(client)

class MigrationTests(unittest.TestCase):
    def test_exact_host_boundary(self):
        with patch.object(client, 'ORIGIN', 'https://ajent.social'):
            for server in ['https://ajent.social', 'https://dogfood.ajent.social']:
                self.assertTrue(client.trusted_server(server))
            for server in ['https://ajent.social.evil.example', 'https://ajent.social@evil.example', 'http://ajent.social', 'https://ajent.social/path', 'https://ajent.social:443', 'https://other.example']:
                self.assertFalse(client.trusted_server(server))
        with patch.object(client, 'ORIGIN', 'https://selfhost.example'):
            self.assertFalse(client.trusted_server('https://ajent.social'))
            self.assertTrue(client.trusted_server('https://selfhost.example'))

    def run_migration(self, destination_id):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / 'credential.json'
            original = {'server': 'https://dogfood.ajent.social', 'token': 'test-only-key'}
            root.write_text(json.dumps(original))
            def request(c, method, route):
                return {'id': 'original' if c['server'] == original['server'] else destination_id}
            with patch.object(client, 'ORIGIN', 'https://ajent.social'), patch.object(client, 'root_path', return_value=root), patch.object(client, 'config_directory', return_value=Path(tmp)), patch.object(client, 'request', side_effect=request):
                if destination_id != 'original':
                    with self.assertRaises(ValueError): client.migrate_credentials()
                    self.assertEqual(json.loads(root.read_text()), original)
                else:
                    client.migrate_credentials()
                    self.assertEqual(json.loads(root.read_text()), dict(original, server='https://ajent.social'))
                    self.assertEqual(root.stat().st_mode & 0o777, 0o600)
                    client.migrate_credentials()  # Retry is idempotent.

    def test_migration_retains_key(self): self.run_migration('original')
    def test_migration_rejects_different_identity(self): self.run_migration('different')

if __name__ == '__main__': unittest.main()
