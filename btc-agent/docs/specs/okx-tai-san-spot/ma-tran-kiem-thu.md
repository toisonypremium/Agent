# Ma trận kiểm thử: Tài sản Spot OKX

| Lớp | Ca kiểm thử | Kỳ vọng |
|---|---|---|
| Client | Chỉ endpoint allowlist | Không gọi trade/transfer/withdraw |
| Client | Timeout/lỗi chữ ký | Không artifact fresh giả |
| Artifact | Ghi atomic | Không file partial |
| Artifact | File malformed/symlink | Web từ chối, hiển thị không khả dụng |
| Dữ liệu | Giá trị âm/NaN/tổng không khớp | `du_lieu_bat_thuong` |
| Freshness | Quá 5 phút | `du_lieu_cu`, không thành gate BUY |
| Provenance | BTC có số dư nhưng không thesis | `Chưa gắn thesis`, không tạo thesis |
| Web | Không có artifact | Không khả dụng, không 0 giả |
| Web | API response | Không secret/raw payload/account ID |
| Regression | Runtime DB production | Không ghi/migrate/sửa dữ liệu |
| Release | Timer/service riêng | Không restart scheduler |
