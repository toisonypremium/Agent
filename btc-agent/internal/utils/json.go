package utils

import "encoding/json"

func Pretty(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
