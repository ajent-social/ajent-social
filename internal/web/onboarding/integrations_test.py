import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('integrations',Path(__file__).with_name('ajent_integrations.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class IntegrationConfigTests(unittest.TestCase):
    def test_preserve_and_repeat(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'mcp.json';original={'other':True,'mcpServers':{'existing':{'command':'keep-me'}}};p.write_text(json.dumps(original))
            module.merge_server(p,'mcpServers',{'command':'python3','args':['client.py','mcp']})
            out=json.loads(p.read_text());self.assertEqual(out['mcpServers']['existing'],original['mcpServers']['existing']);self.assertTrue(out['other'])
            self.assertEqual(len(list(Path(d).glob('*.ajent-backup-*'))),1)
            module.merge_server(p,'mcpServers',{'command':'python3','args':['client.py','mcp']})
            self.assertEqual(len(list(Path(d).glob('*.ajent-backup-*'))),1)
    def test_conflict_and_comments_preserved(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'mcp.json'
            for content in ['{"mcpServers":{"ajent":{"command":"other"}}}', '// keep comments\n{}']:
                p.write_text(content)
                with self.assertRaises(ValueError):module.merge_server(p,'mcpServers',{'command':'python3'})
                self.assertEqual(p.read_text(),content)
    def test_symlink_preserved(self):
        with tempfile.TemporaryDirectory() as d:
            target=Path(d)/'original';target.write_text('{}');p=Path(d)/'mcp.json';p.symlink_to(target)
            with self.assertRaises(ValueError):module.merge_server(p,'mcpServers',{'command':'python3'})
            self.assertEqual(target.read_text(),'{}')
if __name__=='__main__':unittest.main()
