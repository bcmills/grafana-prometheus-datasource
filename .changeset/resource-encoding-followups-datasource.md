---
'grafana-prometheus-datasource': patch
---

Ignore custom Accept-Encoding headers and error on undecodable resource responses. Resource calls only ever request gzip; a response compressed with `deflate` or `br` now surfaces as an error instead of being decoded.
