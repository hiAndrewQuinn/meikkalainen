# meikkalainen
Coarse grained Debian diffs

## Quickstart

Given:

- You can `ssh someone@somewhere` into two separate Debian machines, and
- Your `someone` at both addresses has `sudo` rights,

`meikkalainen` will work its magic by doing

```bash
meikkalainen someone@debian-1 someone-else@debian-2
```

Specifically, `meikkalainen` will dump 2 JSON files of coarse-grained machine details to `json/debian-1/someone_YYYY_MM_DD_HH_MM_SS.json` and `json/debian-2/someone_YYYY_MM_DD_HH_MM_SS.json` respectively, and then semantically diff them. This is the "usual" operation, but indeed, you can also run it with a single

```bash
meikkalainen someone@debian-1                    # JSON dump only
```

Currently the JSON looks like this - there's no schema yet, because this could still use some work.

```json

{
    "timestamp": "2024-04-04T08:39:59.712473827+03:00",
    "debian_version": "12.5\n",
    "architecture": "x86_64",
    "kernel_version": "6.1.0-18-amd64",
    "installed_modules": [
        "ac",
        "aesni_intel",
        "ahci",
        "vmwgfx",
        ...
        "wmi",
        "x_tables"
    ],
    "network_config": {
        "ip_addresses": [
            "10.0.2.15"
        ],
        "interfaces": [
            "eth0",
            "lo"
        ],
        "routing_info": "default via 10.0.2.2 dev eth0 \n10.0.2.0/24 dev eth0 proto kernel scope link src 10.0.2.15"
    },
    "units": [
        {
            "name": "-.mount",
            "load_state": "loaded",
            "active_state": "active",
            "description": "Root Mount"
        },
        ...
        {
            "name": "vagrant.mount",
            "load_state": "loaded",
            "active_state": "active",
            "description": "/vagrant"
        },
    ],
    "libraries": [
        {
            "name": "adduser",
            "version": "3.134"
        },
        ...
        {
            "name": "zlib1g:amd64",
            "version": "1:1.2.13.dfsg-1"
        }
    ]
}
```


or even run multiple dumps with something like

```bash
echo "someone@debian-1" > targets.ssh 
echo "someone-else@debian-2" >> targets.ssh
echo "someone-else-else@debian-3" >> targets.ssh

cat targets.ssh | xargs -n 1 meikkalainen
```

## Motivation

I was working on a lot of Debian systems around the time I wrote `meikkalainen`, and I was running into some problems that I realized I really could have avoided from the start if I had done some better due diligence on making sure the _broad strokes_ of the machines were the same -- simple things like, "Are these both on the same Debian version?", or "Do these both have the same versions of all the same packages installed?" Since I was already learning Go in fits and starts at the time, and had gotten pretty handy with virtual machines, I decided one morning before work to start casting my own problem into this form.
