import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from types import SimpleNamespace

spec=importlib.util.spec_from_file_location('join',Path(__file__).with_name('join.py'))
client=importlib.util.module_from_spec(spec);spec.loader.exec_module(client)

class ScopeTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.base=Path(self.tmp.name).resolve();self.repo=self.base/'repo';self.repo.mkdir();(self.repo/'.git').mkdir()
        self.user=self.base/'user';self.user.mkdir()
        self.env=patch.dict(os.environ,{'AJENT_CONFIG_DIR':str(self.user),'AJENT_CREDENTIAL_FILE':str(self.user/'credential.json')});self.env.start();self.addCleanup(self.env.stop)
        self.cwd=patch.object(Path,'cwd',return_value=self.repo);self.cwd.start();self.addCleanup(self.cwd.stop)
        self.root={'token':'fixture-token','server':'https://fixture.test'}
        (self.user/'credential.json').write_text(json.dumps(self.root))
        self.identity=hashlib.sha256(self.root['token'].encode()).hexdigest()[:16]
    def save(self,path,value):
        path.parent.mkdir(exist_ok=True);path.write_text(json.dumps(value))
    def grants(self):
        self.save(self.user/'scope-grants.json',{self.identity:{'user':[{'id':'u','domain':'user.test'}],client.project_scope():[{'id':'p','domain':'project.test'}]}})
    def test_repo_subdirectories_share_identity(self):
        expected=client.project_profile('codex');nested=self.repo/'src';nested.mkdir()
        with patch.object(Path,'cwd',return_value=nested):
            self.assertEqual(client.project_root(),self.repo)
            self.assertEqual(client.project_profile('codex'),expected)
    def test_project_default_inherits_both_grants(self):
        self.grants();self.save(self.user/'config.json',{'domain':'user.test'})
        self.assertEqual(client.scope_settings()['domain'],'user.test')
        self.save(self.repo/'.aj'/'config.json',{'domain':'project.test'})
        self.assertEqual(client.scope_settings()['domain'],'project.test')
        self.assertEqual(client.scope_settings()['grants'],['p','u'])
    def test_private_override_drops_all_grants(self):
        self.grants();self.save(self.user/'config.json',{'domain':'user.test'})
        self.save(self.repo/'.aj'/'config.json',{'private_only':True})
        self.assertEqual(client.scope_settings()['grants'],[])
        self.assertEqual(client.scope_settings()['domain'],'')
    def test_copied_repo_cannot_grant_authority(self):
        self.save(self.repo/'.aj'/'config.json',{'domain':'foreign.test','grants':['forged']})
        with self.assertRaisesRegex(ValueError,'unapproved'):client.scope_settings()
    def test_account_switch_cannot_reuse_grants(self):
        self.grants();self.save(self.user/'config.json',{'domain':'user.test'})
        self.save(self.user/'credential.json',{'token':'other','server':'https://fixture.test'})
        with self.assertRaisesRegex(ValueError,'unapproved'):client.scope_settings()
    def test_symlink_config_rejected(self):
        target=self.base/'config';target.write_text('{}');(self.user/'config.json').symlink_to(target)
        with self.assertRaisesRegex(ValueError,'symlink'):client.scope_settings()
    def test_existing_profile_reconciles_changed_settings(self):
        client._synced_profiles.clear()
        self.grants();self.save(self.user/'config.json',{'domain':'user.test'})
        calls=[]
        def request(root,method,path,body):
            calls.append(body);return {'agent_id':'agent','group_id':'group'}
        with patch.object(client,'request',side_effect=request):
            first=client.tool_credentials('codex');client.tool_credentials('codex')
            self.assertEqual(len(calls),1)
            self.save(self.repo/'.aj'/'config.json',{'private_only':True})
            second=client.tool_credentials('codex')
        self.assertEqual(first['token'],second['token']);self.assertTrue(calls[-1]['private_only'])
        credential=next((self.user/'profiles').glob('*/*.json'))
        self.assertEqual(credential.stat().st_mode & 0o777,0o600)
        self.assertNotIn('token',(self.repo/'.aj'/'config.json').read_text())

    def test_legacy_settings_do_not_clear_manual_association(self):
        self.assertEqual(client.scope_settings(),{})
    def test_revoked_grant_falls_back_privately_without_minting(self):
        client._synced_profiles.clear();self.grants();self.save(self.user/'config.json',{'domain':'user.test'})
        calls=[]
        def request(root,method,path,body):
            calls.append((path,body))
            if body.get('grants'):raise client.RequestError(403)
            return {'agent_id':'agent','group_id':'group'}
        with patch.object(client,'request',side_effect=request):
            result=client.tool_credentials('codex')
        self.assertEqual([path for path,_ in calls],['/v1/fleet','/v1/fleet'])
        self.assertTrue(calls[-1][1]['private_only'])
        self.assertIn('private collaboration',result['domain_notice'])
    def test_project_configuration_stores_no_credentials(self):
        calls=[]
        def request(root,method,path,body=None):
            calls.append(path)
            if path=='/v1/domains':return {'domains':[{'domain':'project.test','verified':True}]}
            return {'id':'approved','domain':body['domain'],'scope':body['scope']}
        args=SimpleNamespace(scope='project',domain='project.test',private=False,command='configure')
        with patch.object(client,'request',side_effect=request),patch.object(client,'prompt',side_effect=AssertionError('unexpected prompt')):
            client.configure_scope(args,self.root)
        settings=json.loads((self.repo/'.aj'/'config.json').read_text())
        self.assertEqual(settings,{'version':1,'domain':'project.test','private_only':False})
        self.assertEqual(client.scope_settings()['grants'],['approved'])
        self.assertNotIn('fixture-token',(self.repo/'.aj'/'config.json').read_text())
        self.assertEqual((self.user/'scope-grants.json').stat().st_mode & 0o777,0o600)
    def test_unattended_new_domain_prints_proof_and_checks_it(self):
        calls=[]
        def request(root,method,path,body=None):
            calls.append((path,body))
            if path=='/v1/domains':return {'domains':[]}
            if path=='/v1/domains/proof':return {'name':'_ajent-verification.new.test','value':'ajent-verification=public-proof'}
            return {'id':'approved','domain':body['domain'],'scope':body['scope']}
        args=SimpleNamespace(scope='user',domain='new.test',private=False,command='configure')
        with patch.object(client,'request',side_effect=request),patch.object(client,'interactive_terminal',return_value=False):
            client.configure_scope(args,self.root)
        self.assertEqual([body['action'] for path,body in calls if path=='/v1/domains/proof'],['challenge','check'])
        self.assertEqual(client.scope_settings()['domain'],'new.test')

if __name__=='__main__':unittest.main()
