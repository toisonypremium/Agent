# Đặc tả: Quan sát tài sản Spot OKX

## Mục tiêu

Cung cấp cho Web Console một ảnh chụp chỉ đọc về tài sản Spot tại OKX: số dư
khả dụng, đang khóa, tổng số dư, độ mới dữ liệu và cảnh báo đối soát.

Tài sản tại OKX là quan sát sàn. Nó không phải vốn DCA, không phải thesis và
không tự cấp quyền thực thi.

## Phạm vi

- Chỉ đọc số dư Spot OKX bằng credential read-only riêng.
- Xuất artifact allowlist cố định cho Web Console.
- Hiển thị UI tiếng Việt: nguồn, thời điểm, độ mới, số dư và cảnh báo.
- Phân biệt rõ tài sản OKX với phân bổ theo chiến lược.

## Ngoài phạm vi

- Đặt/hủy/sửa lệnh, chuyển/rút tiền, thay config hoặc reservation.
- Futures, Margin, Swap, Earn, đòn bẩy, short.
- Gán thesis từ symbol, số dư coin hoặc lịch sử hiển thị.
- Tự sửa ledger, tự bán coin, hoặc bật auto-live.
- Hiển thị credential, account identifier nhạy cảm hoặc raw API payload.

## Người dùng và tình huống

### Người vận hành

Xem được tài sản Spot tại OKX và biết rõ dữ liệu có fresh hay không.

### Người xem

Xem được cùng read model, nhưng không có quyền halt hoặc mutation.

### Tình huống lỗi

Nếu API lỗi, artifact thiếu/malformed/stale hoặc có dữ liệu bất thường, UI hiển
thị `Không khả dụng` hoặc `Dữ liệu cũ`; không lấy số cũ làm dữ liệu mới.

## Tiêu chí chấp nhận

- AC-01: Observer chỉ gọi endpoint read-only đã allowlist cho Spot.
- AC-02: Artifact có schema version, source, observed_at, freshness, trạng thái.
- AC-03: Artifact không chứa secret/raw request/raw response.
- AC-04: Coin chưa gắn thesis hiển thị `Chưa gắn thesis`.
- AC-05: Không endpoint Web mutation mới.
- AC-06: Không có suy luận `thesis_id` từ symbol.
- AC-07: UI do tính năng này cung cấp dùng tiếng Việt.
- AC-08: Stale quá ngưỡng không được làm đầu vào cho gate BUY tương lai.
- AC-09: Coin có ticker `*-USDT` hợp lệ hiển thị giá trị USDT và tỷ trọng; coin thiếu giá không được gán 0.
- AC-10: Giao diện mobile hiển thị card danh mục, không dùng bảng tràn ngang.
- AC-11: Tất cả test dùng fixture/temp directory/temp SQLite.
