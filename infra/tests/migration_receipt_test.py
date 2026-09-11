import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('migration', Path(__file__).resolve().parents[2] / 'scripts/check-infra-migration.py')
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)


class MigrationReceiptTests(unittest.TestCase):
    def receipt(self):
        return {'changeSummary': {'same': 2}, 'steps': [
            {'op': 'same', 'urn': name, 'newState': {'type': 'test:resource:Resource', 'provider': 'provider-v1', 'protect': True}}
            for name in ['one', 'two']
        ]}

    def test_accepts_complete_unchanged_receipts(self):
        self.assertEqual(migration.compare(self.receipt(), self.receipt()), 2)

    def test_rejects_incomplete_receipt(self):
        receipt = self.receipt()
        receipt['steps'].pop()
        with self.assertRaises(ValueError):
            migration.identities(receipt)

    def test_rejects_changes_and_errors(self):
        for operation in ['create', 'delete', 'update', 'replace', 'create-replacement']:
            with self.subTest(operation=operation):
                receipt = self.receipt()
                receipt['changeSummary'][operation] = 1
                with self.assertRaises(ValueError):
                    migration.identities(receipt)
        receipt = self.receipt()
        receipt['diagnostics'] = [{'severity': 'error'}]
        with self.assertRaises(ValueError):
            migration.identities(receipt)

    def test_rejects_identity_provider_or_protection_change(self):
        for field, value in [('urn', 'renamed'), ('provider', 'provider-v2'), ('protect', False)]:
            with self.subTest(field=field):
                before = self.receipt()
                after = copy.deepcopy(before)
                if field == 'urn':
                    after['steps'][0][field] = value
                else:
                    after['steps'][0]['newState'][field] = value
                with self.assertRaises(ValueError):
                    migration.compare(before, after)
