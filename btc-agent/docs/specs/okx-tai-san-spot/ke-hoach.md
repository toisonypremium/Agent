# Kế hoạch: Quan sát tài sản Spot OKX

## Phương án được chọn

```text
OKX read-only observer → artifact fixed allowlist → typed Web API → UI tiếng Việt
```

Observer sở hữu kết nối credential và artifact. Web Console không mang
credential OKX và không gọi OKX trực tiếp.

## Phương án loại bỏ

### Web Console gọi OKX trực tiếp

Loại bỏ vì mở rộng authority, buộc Web mang credential/proxy authority và làm UI
phụ thuộc trực tiếp vào sàn.

### Ghi snapshot OKX vào thesis ledger

Loại bỏ vì trộn sự thật sàn với vốn chiến lược, phá provenance và có thể tạo
quyền DCA không hợp lệ.

## Lát triển khai

1. Spec và ADR được duyệt.
2. Client OKX read-only với interface fakeable.
3. Schema artifact + writer atomic 0600.
4. Tests artifact/client/validation bằng fixture.
5. Observer service/timer riêng.
6. Typed API + UI tiếng Việt.
7. Read-only staging/deploy verification.
8. Chỉ sau đó mới thiết kế reconciliation gate; không bật auto-live.

## Boundary quyền hạn

| Thành phần | Được làm | Cấm làm |
|---|---|---|
| OKX asset observer | Đọc, chuẩn hóa, ghi artifact | Trade, transfer, withdraw, sửa DB runtime |
| Reconciliation | Phát hiện/chặn/báo incident | Tự sửa ledger hoặc tự SELL |
| Web Console | Đọc và trình bày | Gọi OKX, tạo allocation, đặt lệnh |
| Execution | Đặt limit BUY khi tất cả gate hợp lệ | Suy luận thesis/số dư hoặc bypass halt |
