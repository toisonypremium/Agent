# Mocking guidance

Mock only external seams: exchange transport, clock, filesystem, or provider.
Prefer a fake that models contract state over ordered call expectations. Never
mock storage internals when a temp SQLite DB can exercise the real transaction.
