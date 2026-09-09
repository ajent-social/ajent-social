import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('usage_report',Path(__file__).with_name('usage-report.py'))
report=importlib.util.module_from_spec(spec);spec.loader.exec_module(report)

class UsageReportTests(unittest.TestCase):
    def event(self, **fields):
        return {'msg':'request','schema_version':1,'route':'GET /v1/search','status':200,
                'duration_ms':10,'traffic':'application','client':'mcp','release':'test',**fields}

    def test_metrics_and_candidates(self):
        events=[self.event(result_count=0) for _ in range(5)]
        events += [self.event(status=503,error_code='search_unavailable',duration_ms=100) for _ in range(3)]
        events += [self.event(route='GET /healthz',traffic='health') for _ in range(20)]
        events += [self.event(route='POST /v1/posts',status=201,post_kind='finding',reply=True,structured=True),
                   self.event(route='POST /v1/posts',status=201,replayed=True)]
        r=report.summarize(events)
        self.assertEqual(r['totals']['requests'],30)
        self.assertEqual(r['totals']['non_health_asset_requests'],10)
        self.assertEqual(r['totals']['posts_created'],1)
        self.assertEqual(r['totals']['idempotent_replays'],1)
        search=next(x for x in r['routes'] if x['route']=='GET /v1/search')
        self.assertEqual(search['empty_result_rate'],1)
        self.assertEqual(search['server_error_rate'],3/8)
        self.assertEqual(search['p95_ms'],100)
        self.assertEqual(len(r['candidates']),2)

    def test_export_and_legacy_coverage(self):
        legacy='2026/09/07 04:21:30 INFO request request_id=abc method=GET route="" status=200 duration_ms=0'
        text=json.dumps({'events':[{'message':json.dumps(self.event())},{'message':legacy},{'message':'irrelevant private content'}]})
        r=report.summarize(report.records(text))
        self.assertEqual(r['totals']['requests'],2)
        self.assertEqual(r['totals']['unattributed_requests'],1)
        self.assertEqual(r['totals']['legacy_requests'],1)
        self.assertNotIn('private content',json.dumps(r))

    def test_comparable_rates(self):
        old=report.summarize([self.event(status=503) for _ in range(5)])
        new=report.summarize([self.event() for _ in range(10)])
        change=report.compare(new,old)[0]
        self.assertEqual(change['server_error_rate_delta'],-1)
        self.assertEqual(change['baseline_requests'],5)
        self.assertEqual(change['current_requests'],10)

    def test_no_evidence_is_not_zero_usage(self):
        with tempfile.TemporaryDirectory() as directory:
            p=Path(directory)/'empty.jsonl';p.write_text('')
            result=subprocess.run([sys.executable,str(Path(__file__).with_name('usage-report.py')),'--input',str(p)],capture_output=True)
            self.assertNotEqual(result.returncode,0)
            self.assertEqual(result.stdout,b'')

    def test_invalid_duration(self):
        r=report.summarize([self.event(duration_ms=float('nan'))])
        self.assertEqual(r['totals']['invalid_records'],1)
        self.assertNotIn('requests',r['totals'])

    def test_partial_panic_and_slow_requests(self):
        r=report.summarize([self.event(error_code='panic')] + [self.event(duration_ms=1500) for _ in range(20)])
        self.assertEqual(r['totals']['panics'],1)
        self.assertEqual({x['signal'] for x in r['candidates']},{'panic','slow_p95'})

if __name__=='__main__':unittest.main()
