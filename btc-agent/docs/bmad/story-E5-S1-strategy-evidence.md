# E5-S1 — Kiểm chứng chiến lược bằng dữ liệu lịch sử

## Trạng thái

In progress

## Mục tiêu

Đánh giá chiến lược và live-manager bằng dữ liệu thực tế trước khi xem xét bất kỳ thay đổi nào đối với sizing hoặc market authority.

## Phạm vi

- Backtest BTC gate và Agent 2.
- Live-manager historical simulation.
- Real-data survey.
- Learning report-only.
- Post-check live-auto audit và heartbeat.

## Ngoài phạm vi

- Không sửa threshold trong cùng story.
- Không bật real-order authority.
- Không thay đổi live config.
- Không đặt hoặc hủy lệnh.

## Acceptance criteria

1. Các command kết thúc với exit code 0.
2. Báo cáo có giai đoạn dữ liệu, số mẫu và blocker rõ ràng.
3. Không có NaN, panic hoặc dữ liệu thiếu không được giải thích.
4. Kết quả tách in-sample khỏi walk-forward/out-of-sample nếu report hỗ trợ.
5. Sau khi chạy, scheduler và heartbeat vẫn healthy.
6. `live-auto-audit` không tự nâng quyền vì kết quả backtest.
7. Evidence được lưu tại `reports/bmad-evidence/`.

## Quality gate

```bash
go test ./...
go vet ./...
./scripts/load-env-and-run.sh backtest --config config.yaml
./scripts/load-env-and-run.sh backtest-live-manager --config config.yaml
./scripts/load-env-and-run.sh real-data-survey --config config.yaml
./scripts/load-env-and-run.sh learn --config config.yaml
./scripts/load-env-and-run.sh live-auto-audit --config config.yaml
./scripts/load-env-and-run.sh scheduler-heartbeat-check --config config.yaml
```

## Quyết định sau evidence

- PASS: đủ dữ liệu, kết quả ổn định và không có blocker nghiêm trọng; chuyển sang review evidence.
- NEEDS_MORE_DATA: số mẫu hoặc coverage chưa đủ; tiếp tục thu thập, không tune.
- REJECT: expectancy/drawdown/false-positive không đạt; tạo story nghiên cứu riêng, không sửa production trực tiếp.

## Rollback

Story chỉ đọc dữ liệu và ghi report. Xóa report evidence nếu cần; không có runtime change để rollback.
