<!-- Copyright 2026 Future Gadget Laboratories.

Licensed under the Apache License, Version 2.0. See licenses/APL.txt. -->

# PAR-005 EXPORT conformance

**PENDING:** Record CSV bytes and PARQUET metadata from the stock v23.2.15
binary and compare them with the OSS candidate. Stock-binary oracle recordings
are not yet available; no oracle output is asserted here. The M-BULK export
round-trip comparison is also pending.

The capability checks exercise CSV output and a gzip PARQUET value round trip,
then check the administrator disable and multi-statement transaction errors.
