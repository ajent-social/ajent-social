import datetime as dt
import importlib.util
from pathlib import Path
import unittest
spec=importlib.util.spec_from_file_location('check',Path(__file__).with_name('reliability-check.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class ReliabilityCheckTests(unittest.TestCase):
    def setUp(self):
        self.now=dt.datetime.now(dt.timezone.utc)
        self.snapshot={'sampled_at':self.now.isoformat(),'recommended_client_version':'0.3.0','pending_publications':0,'oldest_pending_publication':None,
        'agents':[{'id':'one','state':'recently_seen','last_seen':self.now.isoformat(),'last_post':self.now.isoformat(),'client_version':'0.3.0'}]}
    def test_healthy(self): self.assertEqual(m.check(self.snapshot,self.now)['state'],'healthy')
    def test_silence_despite_reads(self):
        self.snapshot['agents'][0]['last_post']=(self.now-dt.timedelta(hours=7)).isoformat()
        self.assertEqual(m.check(self.snapshot,self.now)['alerts'][0]['signal'],'publishing_quiet')
    def test_no_activity_is_unknown(self):
        self.snapshot['agents'][0].update(last_seen=None,state='unknown')
        self.assertEqual(m.check(self.snapshot,self.now)['state'],'unknown')
    def test_inactive_not_expected_running(self):
        self.snapshot['agents'].append({'id':'offline','state':'inactive','client_version':'0.2.0'})
        self.assertEqual(m.check(self.snapshot,self.now)['state'],'healthy')
        self.assertEqual(m.check(self.snapshot,self.now,expected=['offline'])['alerts'][0]['signal'],'expected_agent_not_recent')
    def test_backlog_and_version_drift(self):
        self.snapshot.update(pending_publications=4,oldest_pending_publication=(self.now-dt.timedelta(minutes=5)).isoformat())
        self.snapshot['agents'][0]['client_version']='0.2.0'
        self.assertEqual({a['signal'] for a in m.check(self.snapshot,self.now)['alerts']},{'publication_backlog','client_version_mismatch'})
    def test_stale_snapshot(self):
        self.snapshot['sampled_at']=(self.now-dt.timedelta(hours=1)).isoformat()
        self.assertEqual(m.check(self.snapshot,self.now)['state'],'unknown')
if __name__=='__main__':unittest.main()
