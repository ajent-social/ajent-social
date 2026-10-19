"""Extract only the immutable rolling host AMI from a Pulumi stack export."""
import json
import re
import sys


def immutableAMI(value):
    return isinstance(value, str) and re.fullmatch(r"ami-(?:[a-f0-9]{8}|[a-f0-9]{17})", value) is not None


def deployed_host_ami(checkpoint):
    try:
        resources = checkpoint["deployment"]["resources"]
        if not isinstance(resources, list) or any(not isinstance(r, dict) for r in resources):
            raise ValueError()
        matches = [r for r in resources
                   if r.get("type") == "aws:ec2/launchTemplate:LaunchTemplate"
                   and isinstance(r.get("urn"), str)
                   and r["urn"].split("::")[-1] == "application-fleet"]
        if len(matches) != 1:
            raise ValueError()
        template = matches[0]
        if template.get("delete") or template.get("pendingReplacement"):
            raise ValueError()
        ami = template["inputs"]["imageId"]
        if not immutableAMI(ami):
            raise ValueError()
        return ami
    except (KeyError, TypeError, ValueError):
        raise ValueError("Checkpoint must contain exactly one active application-fleet launch template with an immutable AMI") from None


def main():
    try:
        with open(sys.argv[1], encoding="utf-8") as stream:
            ami = deployed_host_ami(json.load(stream))
    except (IndexError, OSError, ValueError, TypeError):
        # Never echo checkpoint contents or parser exceptions containing input.
        print("Unable to derive deployed host AMI from checkpoint", file=sys.stderr)
        return 1
    print(ami)
    return 0


if __name__ == "__main__":
    sys.exit(main())
