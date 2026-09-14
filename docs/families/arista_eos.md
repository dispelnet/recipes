# arista_eos

Six of the seven questions DispelNet asks. Every recipe here reads a real device
transcript from [ntc-templates](https://github.com/networktocode/ntc-templates)
at commit `c6dca50` (Apache-2.0), and none has been run against an Arista device
by this project — which is why no file declares `verified:`. See `NOTICE` at the
repository root.

## bgp-neighbors is missing on purpose

`bgp-neighbors` must produce an `identity`, and identity for a BGP peer is its
router-id: that is what lets a crawl recognise a peer as a device it has already
visited.

`show ip bgp summary` does not report it. The `Router identifier` line is the
*local* device's — which is what `router_id.yaml` reads it for — and the peer
columns give an address and an AS number. `show ip bgp detail` carries a
per-path originator id, which is a property of a route rather than of a peer,
and dumping a routing table to identify neighbours is not a trade worth making.

The command that would answer this is `show ip bgp neighbors`, for which the
corpus has no transcript.

**That is a gap in evidence, not in the format.** A recipe is a graph, so this
is writable as two steps: `show ip bgp summary` for the peers, then
`show ip bgp neighbors <peer>` once per peer for its router-id, joined on the
address — the shape `recipes/nokia_srlinux/addresses.yaml` already uses and
`docs/recipes/format.md` demonstrates with `for-each`. What stops it is that a recipe shipping no
captured output is a recipe nothing can check.

`juniper_junos` had the same gap and no longer does: a real transcript of its
per-peer command turned up in suzieq's test data, so the recipe reads one
command and needs no graph at all. The same search does not rescue EOS — suzieq
polls Arista over eAPI, so its CLI transcripts for `show ip bgp neighbors` are
empty.

Until someone captures that command from an EOS device, a walk records a gap for
`bgp-neighbors` — the honest report: nobody here can ask it, rather than the
device having no peers.
