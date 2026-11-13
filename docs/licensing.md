# Licensing and redistribution

## Project license

Needle-X is released under the **MIT License**. The canonical text and SPDX
identifier are in [LICENSE](../LICENSE); the copyright holder is Josè Pavese.

This declaration covers the source repository and the native release artifacts
from **v0.1.47 onward**. Earlier public tags (through v0.1.46) were published
without license and notice material; they are superseded by v0.1.47 and should
not be redistributed.

## What ships in a native archive

Every `needlex_<os>_<arch>` archive contains:

- `needlex` (or `needlex.exe`)
- `LICENSE` — the project MIT license and copyright notice
- `THIRD_PARTY_NOTICES.txt` — the license and notice texts of the Go modules
  linked into the binary

The archive check is enforced in two places:

- `scripts/release/build_release.sh` runs
  `scripts/release/check_archive_licenses.sh` for every archive it builds;
- `.github/workflows/dist.yml` runs the same check in the build matrix and again
  on the downloaded artifacts before publishing a GitHub release.

A failed check rejects the archive instead of publishing an incomplete release.

## Third-party components

Needle-X links third-party Go modules. Their licenses are compatible with
redistribution under the project license and are reproduced in
`THIRD_PARTY_NOTICES.txt`; the file lists each module and version and groups
the license texts by content. Do not edit that file by hand. Regenerate it
after changing `go.mod` or the modules linked by `cmd/needle`:

```bash
go mod download
python3 scripts/release/generate_third_party_notices.py
```

The generator reads the modules linked into `./cmd/needle`, finds their
license/notice files, reproduces them, and writes the tracked
`THIRD_PARTY_NOTICES.txt`.

## Redistribution expectations

If you redistribute a Needle-X binary, for example inside a container image or
an internal package:

1. keep `LICENSE` and `THIRD_PARTY_NOTICES.txt` with the binary;
2. do not remove, replace, or narrow the copyright and license notices;
3. comply with the third-party terms reproduced in the notices file;
4. do not imply trademark endorsement or project endorsement;
5. keep the alpha-status warning visible until the project leaves alpha.

The MIT license grants no trademark rights and provides the software “as is”,
without warranty. Downstream distributions remain responsible for their own
compliance with the licenses of the components they ship.

## Why the license was not declared earlier

The project was published as an alpha without a license file. An external
consumer report (GitHub issue #2) noted that downstream distributions with a
declared-license requirement could not include the binary. v0.1.47 adds the
license, the notices and the release checks; the issue records the evidence.
