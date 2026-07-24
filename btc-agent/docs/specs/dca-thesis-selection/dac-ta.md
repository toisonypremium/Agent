# Đặc tả: Danh sách DCA theo thesis

## Mục tiêu

Làm rõ coin đang nghiên cứu, coin được phép thành thesis DCA, vốn được cấp và mọi blocker thực thi.

## Quyết định chiến lược

- Spot-only, DCA-only; không Market BUY, Futures/Margin, leverage, short hay automatic stop-loss SELL.
- Thesis-first: coin không có `thesis_id` không có quyền DCA.
- Vốn DCA là USDT được cấp trong thesis, không phải toàn bộ tài khoản Spot.
- DCA theo tín hiệu với limit/post-only ba lớp: 25% / 35% / 40% max exposure thesis.
- `HALTED_SHADOW` vẫn chặn toàn bộ execution.

## Danh sách DCA được chỉ định

| Coin | Vai trò nghiên cứu | Rủi ro riêng | Trạng thái |
|---|---|---|---|
| ETH | Core smart-contract liquidity | Beta thị trường, cạnh tranh roadmap | Thuộc danh sách DCA; chưa cấp thesis |
| LINK | Oracle / CCIP infrastructure | Token value capture, supply/dilution, adoption | Thuộc danh sách DCA; chưa cấp thesis |
| VIRTUAL | AI-agent protocol narrative | Biến động cao, tokenomics/narrative risk | Thuộc danh sách DCA rủi ro cao; chưa cấp thesis |

Danh sách DCA là universe chiến lược đã chỉ định. Nó không phải allowlist execution, không tự tạo `thesis_id` và không tự tạo allocation.

## Cổng vào DCA

Coin trong `danh_sach_dca` chỉ chuyển sang `cho_phe_duyet_thesis` khi tất cả đạt:

- Cặp Spot `COIN-USDT` quan sát được; thanh khoản/spread đạt ngưỡng.
- Luận điểm có ID, timeframe, điều kiện invalidation và evidence source.
- Tokenomics/unlock/dilution được rà soát với timestamp/source rõ.
- Market data và BTC regime đủ mới.
- Không có reconciliation blocker hay incident an toàn chưa xử lý.

Chuyển sang `duoc_cap_von` chỉ sau thao tác operator tạo thesis ledger riêng. Lát hiện tại không có endpoint tạo/sửa thesis.

## Cổng tạo DCA layer

```text
operator_halt = false
runtime = healthy
BTC regime / planner permission = allowed
thesis lifecycle = allows increase
reconciliation = clean
liquidity + spread + orderbook = pass
capital reservation = atomic success
```

Không cổng nào trong tài liệu này bật execution; đây chỉ là read model cho operator.

## Cơ chế ba lớp

| Lớp | Tỷ lệ max exposure thesis | Quy tắc |
|---|---:|---|
| Lớp 1 | 25% | Limit/post-only, tất cả gate đạt |
| Lớp 2 | 35% | Thesis hợp lệ; không averaging mù |
| Lớp 3 | 40% | Drawdown/discount và toàn bộ gate đạt |

Không layer nào dựa trên số coin đang có hoặc map symbol sang thesis.

## Web read model

`GET /api/v1/strategy/dca` là chỉ đọc. Dashboard phải hiển thị: giai đoạn, cơ chế 3 lớp, shortlist, risk tier, vốn thesis được cấp (mặc định 0), và blocker. Không có action mutation.
