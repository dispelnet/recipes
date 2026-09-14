# dispelnet recipes

Recipes tell [`dispelnet`](https://dispelnet.com) which read-only command to run
on a network device and how to turn its output into facts. Probes identify the
device family. Each recipe and probe has a matching `.captured` file containing
the output it was written against, which CI checks it against; releases ship
the recipes and probes alone.

> **Status:** pre-release. No release of this repository has been published
> yet.

```yaml
family:   frr
question: bgp-neighbors
applies:  ["10.*"]

steps:
  - run: vtysh -c "show bgp neighbors json"
    select: "{*}"
    key:    address
    fields:
      identity: remoteRouterId
      as:       remoteAs
      state:    {from: bgpState, lower: true}
```

## Layout

```text
recipes/<family>/<question>.yaml        recipe
recipes/<family>/<question>.captured    captured device output
recipes/_probes/<family>.yaml           identification probe
recipes/_probes/<family>.captured
recipes/_questions/<question>.json      the fields a question's rows carry
docs/                                   contribution, release and maintainer notes
Makefile                                make check: layout and dispelnet verify
```

## Questions and their fields

A recipe answers a question, and every recipe answering that question produces
the same fields, whatever the platform calls them: FRR's `remoteRouterId` and
SR Linux's `peer-router-id` both land as `identity`. What those fields are, and
what each one holds, is written down per question in `recipes/_questions/` as
JSON Schema:

```json
"port": {
  "type": "string",
  "x-dispelnet-type": "interface",
  "description": "The neighbour's own port, named as the neighbour names it."
}
```

`x-dispelnet-type` names the kind of value, from a set `dispelnet` implements:
`ip`, `mac`, `interface`, `asn`, `router-id`, `hostname`, `state` and `text`.
The type is what a value is normalised by, so that `Gi0/1` and
`GigabitEthernet0/1`, or `0011.2233.4455` and `00:11:22:33:44:55`, are
recognised as the same thing when a far end is matched to a device.

## Contributing

See [Adding device and command support](docs/ADDING_SUPPORT.md) for validation
and contribution guidance.

## Releases

Releases are immutable GitHub releases carrying the recipe tree as a
tarball. [Releases](docs/RELEASES.md) describes what is in one and how to verify
it; [Versioning](docs/VERSIONING.md) says what a version number promises.
Maintainers: [Maintaining](docs/MAINTAINING.md).

## Licence

Apache-2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE).
