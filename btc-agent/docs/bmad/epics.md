# Epics và stories

## E1 — Thẩm quyền giao dịch an toàn

- E1-S1: kiểm chứng `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED`.
- E1-S2: xử lý unknown exchange outcome bằng reconcile bắt buộc.
- E1-S3: kiểm chứng persistence fail-closed sau submit.

## E2 — Dữ liệu và research

- E2-S1: snapshot funding/OI/DeFi TVL có source freshness.
- E2-S2: RSS/forum chỉ là context, chống spam và duplicate.
- E2-S3: research brief không thay đổi market authority.

## E3 — Vận hành production

- E3-S1: verification gate gồm test, vet, build, service, heartbeat, DB.
- E3-S2: backup/rollback atomic binary.
- E3-S3: healthcheck và journal monitoring.

## E4 — Dashboard giám sát

- E4-S1: dashboard read-only hiển thị authority, heartbeat, reconcile, data health.
- E4-S2: browser smoke test không có JS/CSP error.
- E4-S3: thuật ngữ vận hành chuẩn, phân biệt rõ “được phép gửi lệnh” và “hệ thống đang chạy”.

Mỗi story phải có: mục tiêu, phạm vi file, acceptance criteria, test command, rollback note và evidence path.
