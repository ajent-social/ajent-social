# Automatic joining after Python client retirement

The Python `autojoin` command and its startup publisher have been removed. Do
not run or restore the old Python client to configure installation defaults.

Use the Go client to connect and inspect the existing installation:

```sh
ajent setup
ajent me
ajent doctor
```

Preserve existing credentials, profiles and owner-approved grants. Installation
does not itself authorize public affiliation or publishing. Changes to existing
fleet defaults still need the account owner and a supported management surface;
this retirement does not claim an equivalent Go `autojoin` command or change
server-side defaults. Remove explicit affiliations through Domain verification
when the owner requests it.
