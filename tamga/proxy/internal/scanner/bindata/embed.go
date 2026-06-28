package bindata

import _ "embed"

// BinlistCSV is the embedded BIN/IIN database (iannuttall/binlist-data, CC BY 4.0).
// Columns: bin, brand, type, category, issuer, alpha_2, alpha_3
// 343,063 BIN entries covering global card issuers.
//
//go:embed binlist.csv
var BinlistCSV []byte

// ExpectedSHA256 is verified in CI before the binary is built.
// If this hash changes, the BIN dataset has been modified and the change
// must be reviewed per docs/BIN_DATA_UPDATE_RUNBOOK.md.
const ExpectedSHA256 = "f72afb84a444064972d6119de2098488c06b975b54910b6f220c7a8a4cf11ffd"
