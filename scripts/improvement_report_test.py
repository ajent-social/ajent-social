import datetime as dt
import importlib.util
from pathlib import Path
import unittest
s=importlib.util.spec_from_file_location('report',Path(__file__).with_name('improvement-report.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
class ReportTest(unittest.TestCase):
 def test_stale_is_unknown(self):
  self.assertEqual(m.report({'sampled_at':'2026-01-01T00:00:00Z'},dt.datetime(2026,1,2,tzinfo=dt.timezone.utc))['state'],'unknown')
 def test_missing_metrics_not_zero(self):
  self.assertEqual(m.report({'sampled_at':'2026-01-01T00:00:00Z','state':'unknown'},dt.datetime(2026,1,1,tzinfo=dt.timezone.utc))['state'],'unknown')
 def test_local_projection_excludes_context(self):
  r=m.report({'sampled_at':'2026-01-01T00:00:00Z','state':'ok','metrics':{}},dt.datetime(2026,1,1,tzinfo=dt.timezone.utc),{'logical_failures':3,'secret':'PRIVATE'})
  self.assertEqual(r['local_client_counters'],{'logical_failures':3});self.assertNotIn('PRIVATE',str(r))
