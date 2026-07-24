# Hiến pháp auto-live 24/7

## Mục đích

Tài liệu này xác lập các bất biến bắt buộc cho mọi thiết kế, thay đổi mã,
triển khai và phê duyệt liên quan đến vận hành tự động 24/7.

## Bất biến không được vi phạm

1. Chỉ giao dịch Spot; cấm Futures, Margin, Swap, đòn bẩy và short.
2. BUY chỉ limit/post-only; cấm Market BUY.
3. Không tự động SELL cắt lỗ chỉ từ lỗ, invalidation hoặc lifecycle.
4. `thesis_id` là provenance bất biến; không suy ra từ symbol.
5. Tài sản tại OKX và vốn theo chiến lược là hai nguồn sự thật riêng.
6. Coin chưa gắn thesis không tạo quyền BUY, SELL, reserve hoặc release.
7. Unknown outcome luôn giữ vốn bị khóa cho đến khi đối soát thủ công xác nhận.
8. Reservation, fill và release phải nguyên tử, idempotent và có audit.
9. Observer và Web Console chỉ đọc; không được mang quyền đặt/hủy lệnh.
10. Operator halt là một chiều, không tự resume và không bị bypass.
11. Mọi gate lỗi, thiếu, stale hoặc mâu thuẫn đều fail-closed cho BUY mới.
12. Release production phải immutable, kiểm SHA-256, có verifier và rollback rõ.
13. Test không dùng production DB, credential thật hoặc lệnh thật.

## Vòng đời trạng thái auto-live

```text
HALTED_SHADOW
→ SHADOW_READY
→ RECONCILIATION_READY
→ CANARY_PENDING_APPROVAL
→ CANARY_ACTIVE
→ LIMITED_AUTO_LIVE
→ FULL_AUTO_LIVE
```

Không state nào tự chuyển sang state có quyền thực thi. Mỗi transition yêu cầu
spec, evidence, kiểm thử, release immutable và phê duyệt riêng.

## Gate tối thiểu cho BUY trong tương lai

```text
- Spot-only và limit/post-only
- Operator halt không active
- Lease scheduler singleton và còn hạn
- Snapshot OKX fresh
- Không có drift/unknown outcome/incident critical
- Market + planner + risk cho phép
- Thesis explicit, lifecycle cho phép
- Reservation nguyên tử thành công
```

Một gate không đạt phải chặn BUY mới, không suy diễn hoặc tự sửa dữ liệu.
