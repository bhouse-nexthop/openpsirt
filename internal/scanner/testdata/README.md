# Scanner fixtures

`grype-output.json` is **constructed, not recorded.** Its field set and the
values in it come from a consumer that has been run against real output —
`scripts/sbom_vuln_scan.py` in the SONiC build, which reads exactly these
fields — rather than from a scanner run here.

That makes it good evidence of what the fields mean and poor evidence of what
else the output contains. Replacing it with a recorded run is worth doing when
a scanner and its database are available, and the shape it settles is which
fields are read, not whether the reading is complete.
