# TODO.AI.md

- CI `vuln-scan` job (govulncheck) failing on `main` as of commit `f197de572a72` — 7 new Go
  stdlib CVEs (GO-2026-6218, 6091, 6090, 6089, 6088, 5972, 5026), all fixed in go1.26.6;
  `casjaysdev/go:latest` currently ships go1.26.5. Not caused by any code in this repo —
  previous push (`fb9de177`) passed CI cleanly on the same image. Re-run CI once
  `casjaysdev/go:latest` picks up go1.26.6, or track upstream image update; remove this
  item once `vuln-scan` is green again.
