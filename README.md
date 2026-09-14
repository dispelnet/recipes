# DispelNet recipes

Recipes tell [`dispelnet`](https://dispelnet.com) which read-only command to run
on a network device and how to turn its output into facts. Probes identify the
device family. Each recipe and probe has a matching `.captured` file containing
the output it was written against.

```yaml
family:   frr
question: bgp-neighbors
applies:  ["10.*"]
verified: frr 10.3.4
command:  vtysh -c "show bgp neighbors json"
select:   "{*}"
key:      address
fields:
  identity: remoteRouterId
  as:       remoteAs
  state:    {from: bgpState, lower: true}
```

## Layout

```text
recipes/<family>/<question>.yaml        recipe
recipes/<family>/<question>.captured    captured device output
recipes/_probes/<family>.yaml            identification probe
recipes/_probes/<family>.captured
docs/                           contribution, versioning, and family notes
tools/                          provenance and bump checkers (Go)
.dispelnet-version              dispelnet commit or tag checked by CI
```

## Contributing

See [Adding device and command support](docs/ADDING_SUPPORT.md) for validation
and contribution guidance.

Release policy is in [Versioning](docs/VERSIONING.md). The repository is
Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE).
