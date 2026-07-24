# ADR 0004: Tách tài sản Spot OKX khỏi vốn chiến lược

- Trạng thái: Được chấp nhận
- Ngày: 2026-07-24

## Bối cảnh

Bot cần quan sát số dư coin và USDT thực tế tại OKX để đối soát và trình bày.
`thesis_capital_ledger` lại là authority riêng cho reservation, filled capital và
hạn mức theo thesis. Hai dữ liệu có thể khác nhau hợp lệ: tài sản thủ công,
tài sản lịch sử hoặc coin chưa có provenance không được tạo quyền giao dịch.

## Quyết định

Tạo observer tài sản OKX có credential read-only riêng, chỉ gọi endpoint Spot
đã allowlist. Observer xuất artifact immutable/atomic cho Web Console. Web
Console chỉ đọc artifact fixed allowlist và không mang credential OKX.

Artifact tài sản không ghi vào `thesis_capital_ledger`, không tạo thesis, không
thay đổi reservation, không sửa drift và không cấp quyền BUY/SELL.

## Hệ quả

- UI hiển thị độc lập “Tài sản tại OKX” và “Phân bổ theo chiến lược”.
- Coin chưa có thesis hiển thị là chưa gắn thesis và yêu cầu rà soát.
- Snapshot lỗi hoặc stale fail-closed cho các gate BUY tương lai.
- Reconciliation chỉ báo lệch/chặn; không tự repair hoặc tự SELL.
- Cần API key OKX read-only, Spot-only riêng trước deploy production.

## Phương án loại bỏ

### Web Console gọi OKX trực tiếp

Tăng bề mặt credential/authority và làm UI phụ thuộc vào sàn.

### Ghi snapshot OKX vào ledger thesis

Trộn observation và authority, có thể tạo quyền DCA không có provenance.
