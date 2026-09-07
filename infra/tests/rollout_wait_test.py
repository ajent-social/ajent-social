import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('wait_rollout', Path(__file__).resolve().parents[2] / 'scripts/wait-rollout.py')
rollout = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rollout)


class RolloutResultTests(unittest.TestCase):
    def run_result(self, rows):
        with patch('sys.argv', ['wait-rollout.py', 'test-fleet']), patch.object(rollout.subprocess, 'check_output', return_value=json.dumps({'InstanceRefreshes': rows})), patch('builtins.print'):
            rollout.main()

    def test_success(self):
        self.run_result([{'InstanceRefreshId': 'refresh-1', 'Status': 'Successful'}])

    def test_rollback_is_not_a_successful_release(self):
        for status in ['Failed', 'Cancelled', 'RollbackSuccessful', 'RollbackFailed']:
            with self.subTest(status=status), self.assertRaises(SystemExit):
                self.run_result([{'InstanceRefreshId': 'refresh-1', 'Status': status}])

    def test_missing_refresh_cannot_pass(self):
        with self.assertRaises(SystemExit):
            self.run_result([])

    def test_pins_observed_refresh_while_waiting(self):
        rows = [{'InstanceRefreshes': [{'InstanceRefreshId': 'refresh-1', 'Status': status}]} for status in ['InProgress', 'Successful']]
        with patch('sys.argv', ['wait-rollout.py', 'test-fleet']), patch.object(rollout.subprocess, 'check_output', side_effect=[json.dumps(row) for row in rows]) as aws, patch.object(rollout.time, 'sleep'), patch('builtins.print'):
            rollout.main()
        self.assertEqual(aws.call_args_list[1].args[0][-2:], ['--instance-refresh-ids', 'refresh-1'])
