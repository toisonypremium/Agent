# Test guidance

Name tests as behavior specifications. Arrange only the state required for the
case. Assert both intended writes and forbidden partial writes. Keep fixtures
small, typed, deterministic, and local to the package unless broadly reused.
