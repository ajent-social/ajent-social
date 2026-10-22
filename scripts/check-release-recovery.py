#!/usr/bin/env python3
"""Accept only a previous Ajent ASG launch-template version update."""
import json
import sys


def recorded_output_drift(meta, previous, selected):
    """The desired template version is already stored, but the last observed output lags it."""
    if previous != selected or not isinstance(selected, str) or not selected.isdecimal():
        return False
    observed = (meta.get("old") or {}).get("outputs", {}).get("launchTemplate", {}).get("version")
    if not isinstance(observed, str) or not observed.isdecimal() or int(observed) >= int(selected):
        return False
    return meta.get("detailedDiff") == {"launchTemplate.version": {"diffKind": "update", "inputDiff": False}}


def validate(events):
    changes = []
    summaries = []
    for event in events:
        if event.get("diagnosticEvent", {}).get("severity") == "error":
            raise ValueError("preview contains an error")
        if "summaryEvent" in event:
            summaries.append(event["summaryEvent"])
        meta = event.get("resourcePreEvent", {}).get("metadata")
        if not meta or meta.get("op") in ("same", "read", "read-discard"):
            continue
        if meta.get("type") == "pulumi:pulumi:Stack" and meta.get("op") == "update":
            continue
        if meta.get("op") != "update" or meta.get("type") != "aws:autoscaling/group:Group":
            raise ValueError("recovery may update only the application Auto Scaling group")
        if meta.get("urn") != "urn:pulumi:production::ajent-infra::aws:autoscaling/group:Group::application-fleet":
            raise ValueError("unexpected Auto Scaling group")
        old = json.loads(json.dumps(meta.get("old", {}).get("inputs", {})))
        new = json.loads(json.dumps(meta.get("new", {}).get("inputs", {})))
        old_lt, new_lt = old.get("launchTemplate", {}), new.get("launchTemplate", {})
        if old_lt.get("id") != "lt-053e4c8f50d1233a3" or new_lt.get("id") != old_lt["id"]:
            raise ValueError("launch template identity changed")
        previous, selected = old_lt.pop("version", None), new_lt.pop("version", None)
        if recorded_output_drift(meta, previous, selected):
            if old != new:
                raise ValueError("recovery would change inputs other than template version")
            changes.append(meta)
            continue
        if not isinstance(previous, str) or not previous.isdecimal() or not isinstance(selected, str) or not selected.isdecimal() or int(selected) <= int(previous):
            raise ValueError("recovery requires a newer numbered template version")
        if old != new:
            raise ValueError("recovery would change inputs other than template version")
        changes.append(meta)
    if len(changes) != 1 or len(summaries) != 1 or summaries[0].get("maybeCorrupt"):
        raise ValueError("expected exactly one complete version-only update")
    return "Validated one application template-version recovery; no other resource changes."


if __name__ == "__main__":
    try:
        with open(sys.argv[1], encoding="utf-8") as stream:
            print(validate([json.loads(line) for line in stream if line.strip()]))
    except (ValueError, KeyError, TypeError) as exc:
        raise SystemExit(str(exc)) from exc
