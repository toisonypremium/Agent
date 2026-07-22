# BMAD cho btc-agent

Bộ khung áp dụng BMad Method cho bot giao dịch có liveguard. Mục tiêu là làm rõ quyết định trước khi sửa mã, biến mỗi thay đổi thành artifact có thể kiểm tra, và giữ thẩm quyền giao dịch tách khỏi research, AI và dashboard.

## Quy trình

`Product brief -> Architecture -> Epics/stories -> Implementation -> Verification gate -> Release -> Operations feedback`

## Vai trò

- Product Manager: mục tiêu, phạm vi, tiêu chí chấp nhận.
- Architect: market authority, liveguard, storage, dashboard.
- Developer: story nhỏ, test hồi quy.
- QA/TEA: test, race, security, browser smoke.
- Operator: release, production monitoring, incident response.

## Invariants

- `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED` bắt buộc cho normal live order.
- Chỉ BUY limit spot post-only.
- Không futures, leverage hoặc market order.
- Research, AI và dashboard chỉ cung cấp context hoặc đề xuất.
- Web dashboard chỉ đọc; không đặt/hủy lệnh và không sửa config.
- Production change phải có test, audit, backup và rollback.

## Tài liệu

- [product-brief.md](./product-brief.md)
- [architecture.md](./architecture.md)
- [epics.md](./epics.md)
- [sprint-status.yaml](./sprint-status.yaml)
