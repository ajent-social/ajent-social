import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("release_recovery", Path(__file__).with_name("check-release-recovery.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ReleaseRecoveryTest(unittest.TestCase):
    def events(self):
        inputs = {"launchTemplate": {"id": "lt-053e4c8f50d1233a3", "version": "44"}, "desiredCapacity": 2}
        updated = copy.deepcopy(inputs)
        updated["launchTemplate"]["version"] = "45"
        return [{"resourcePreEvent": {"metadata": {"op": "update", "type": "aws:autoscaling/group:Group", "urn": "urn:pulumi:production::ajent-infra::aws:autoscaling/group:Group::application-fleet", "old": {"inputs": inputs}, "new": {"inputs": updated}}}}, {"summaryEvent": {"resourceChanges": {"update": 1, "same": 46}}}]

    def test_accepts_version_only_recovery(self):
        self.assertIn("Validated", module.validate(self.events()))

    def test_rejects_other_effects(self):
        for op in ("create", "delete", "replace", "create-replacement", "delete-replaced", "refresh"):
            with self.subTest(op=op):
                events = self.events()
                events.insert(0, {"resourcePreEvent": {"metadata": {"op": op, "type": "aws:ec2/instance:Instance"}}})
                with self.assertRaises(ValueError): module.validate(events)

    def test_rejects_other_asg_input_changes(self):
        events = self.events()
        events[0]["resourcePreEvent"]["metadata"]["new"]["inputs"]["desiredCapacity"] = 3
        with self.assertRaises(ValueError): module.validate(events)

    def test_rejects_other_template_or_group(self):
        for change in ("template", "group"):
            events = self.events()
            meta = events[0]["resourcePreEvent"]["metadata"]
            if change == "template": meta["new"]["inputs"]["launchTemplate"]["id"] = "lt-other"
            else: meta["urn"] = "other-group"
            with self.assertRaises(ValueError): module.validate(events)

    def test_rejects_unpinned_or_nonforward_version(self):
        for version in ("$Latest", "$Default", "44", "43", None, 45):
            with self.subTest(version=version):
                events = self.events()
                events[0]["resourcePreEvent"]["metadata"]["new"]["inputs"]["launchTemplate"]["version"] = version
                with self.assertRaises(ValueError): module.validate(events)

    def test_rejects_incomplete_duplicate_or_error_preview(self):
        cases = [[], self.events()[:1], self.events() + self.events(), self.events() + [{"diagnosticEvent": {"severity": "error"}}]]
        for events in cases:
            with self.subTest(events=events):
                with self.assertRaises(ValueError): module.validate(events)
