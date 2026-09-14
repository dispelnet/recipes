# juniper_junos

Every question a device can be asked about its neighbours and addresses, plus
`version`.

## Its BGP peers come from a different corpus

ntc-templates carries only `show bgp summary` for Junos, which reports peer
addresses and the **local** router-id. The identity of a BGP peer has to be
*its* router-id, or a crawl cannot recognise a device it has already visited.

`show bgp neighbor | display json` answers it in one command, and a real
transcript of it lives in [suzieq](https://github.com/netenglabs/suzieq)'s test
data -- a `junos-qfx` device, Apache-2.0. `peer-id` is the peer's router-id;
`peer-address` carries a TCP source port, which the recipe trims.

Worth remembering when a platform looks unable to answer something: the
evidence may be in somebody else's repository.
