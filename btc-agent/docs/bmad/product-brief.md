# Product brief

## Vấn đề

btc-agent cần vận hành production có kiểm soát: dữ liệu nhiều nguồn, market gate chặt, order lifecycle có đối soát, và giao diện giúp operator hiểu đúng trạng thái mà không mở thêm quyền giao dịch.

## Kết quả mong muốn

1. Phân biệt rõ monitoring, dry-run và real-order authority.
2. Mọi quyết định live có bằng chứng: doctor, audit, reconcile, risk và microstructure.
3. Research/AI tạo context, không tự nâng quyền.
4. Release có rollback và xác minh sau deploy.

## Ngoài phạm vi

- Không tự xoay credentials trong workflow này.
- Không thêm execution endpoint vào web.
- Không dùng sentiment/forum làm tín hiệu giao dịch độc lập.

## Tiêu chí chấp nhận cấp sản phẩm

- Khi dữ liệu stale, authority giảm hoặc bị chặn.
- Khi exchange outcome không rõ, hệ thống dừng và yêu cầu reconcile.
- Khi persistence sau submit lỗi, không tiếp tục batch.
- Dashboard chỉ đọc và hiển thị nguồn, độ cũ, blocker.
