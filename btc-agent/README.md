# btc-agent

Bot giao dịch spot theo luật cứng cho Termux/root Android. Hệ thống dùng BTC làm market gate, sau đó đánh giá ETH/SOL/RENDER theo setup accumulation riêng. Bot chỉ tạo lệnh spot limit BUY post-only khi mọi gate an toàn cùng pass.

## Trạng thái hiện tại

- Runtime chính: `scheduler` ở `live-auto` -> `live-supervisor` -> managed order engine.
- Gate live bắt buộc: `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED`.
- Khi BTC chưa xác nhận accumulation, bot giữ `WATCH`/`SCOUT`, `desired=0`, không đặt lệnh.
- Dữ liệu futures/microstructure nếu thêm sau này chỉ dùng quan sát, không dùng để thực thi futures.

## Safety invariants

- Config mặc định an toàn: paper/simulation only, live disabled, proof-only enabled, real trading disabled.
- Normal live desired orders chỉ được tạo khi plan `ACTIVE_LIMIT`, BTC permission `ALLOWED`, BTC accumulation phase `ACCUMULATION_CONFIRMED`, asset state `ACTIVE_LIMIT`.
- `WATCH`, `SCOUT`, `ARMED` chỉ là trạng thái quan sát/giải thích. Không có quyền tạo normal live order.
- Chỉ dùng spot limit BUY post-only.
- Không futures, không leverage, không market order.
- Telegram chỉ read-only. Không có lệnh mua/bán/hủy/override qua Telegram.
- Survey, learning, report, dashboard không được sửa config, không đặt lệnh, không bypass Agent 1/2 hoặc live guard.
- Secret không được commit: OKX keys, Telegram token, `.env`, `config.yaml`, DB, logs, reports, binaries.

## Kiến trúc quyết định

```text
BTC market gate
  -> BTC accumulation detector
  -> Agent 1 permission
  -> Agent 2 asset setup
  -> ladder spot-limit planner
  -> liveguard preflight/risk/reconcile/data checks
  -> managed order engine
```

### Agent 1: BTC market gate

Agent 1 phân tích BTC như benchmark thị trường, không xem BTC là asset để gom. BTC được phân loại deterministic từ OHLCV đóng nến:

```text
MARKDOWN
LIQUIDITY_SWEEP
SELL_ABSORPTION
RECLAIM
ACCUMULATION_CONFIRMED
DISTRIBUTION
INVALIDATED
```

Mapping quyền:

- `MARKDOWN`, `LIQUIDITY_SWEEP`, `SELL_ABSORPTION`: tối đa `WATCH`.
- `RECLAIM`: tối đa `ARMED`.
- `ACCUMULATION_CONFIRMED`: được xét tiếp để lên `ALLOWED`, nhưng vẫn phải qua risk/FOMO/data/RR gates.
- `DISTRIBUTION`, `INVALIDATED`: hard block hoặc `NO_TRADE`.

### Agent 2: asset accumulation planner

Agent 2 chỉ đánh giá assets trong `data.symbols.assets`. Setup asset cần pass:

- BTC đã `ACCUMULATION_CONFIRMED`.
- BTC permission `ALLOWED`.
- Asset flow có reclaim/absorption footprint hợp lệ.
- Không falling knife/distribution.
- Discount zone, reward/risk, liquidity, rotation, relative strength đạt chuẩn.
- Data health, risk governor, reconcile safety, live preflight đều pass.

Plan states:

| State | Ý nghĩa | Có quyền tạo normal live order? |
|---|---|---|
| `NO_TRADE` | hard block hoặc setup không dùng được | Không |
| `WATCH` | candidate để theo dõi | Không |
| `SCOUT` | gần setup nhưng còn chờ gate mềm | Không |
| `ARMED` | context mạnh, chưa đủ order authority | Không |
| `ACTIVE_LIMIT` | đủ gate cuối, có layers hợp lệ | Có, nếu mọi live guard pass |

Normal managed live desired orders chỉ được build khi:

```text
plan.State == ACTIVE_LIMIT
plan.ActionPermission == ALLOWED
BTC accumulation phase == ACCUMULATION_CONFIRMED
asset.State == ACTIVE_LIMIT
```

Sau đó vẫn phải pass preflight, data health, reconcile safety, risk governor, notional caps, open-order caps, liquidity/MM checks, post-only checks.

## Cài đặt trên Termux

```bash
pkg update
pkg install golang git ca-certificates
cd /data/data/com.termux/files/home/.openclaw/workspace/btc-agent
go mod tidy
go build -o bin/btc-agent .
cp config.yaml.example config.yaml
```

Không commit file runtime thật:

```text
config.yaml
.env
*.db
reports/
logs/
bin/
```

## Lệnh chính

```bash
./bin/btc-agent fetch --config config.yaml
./bin/btc-agent analyze --config config.yaml
./bin/btc-agent plan --config config.yaml
./bin/btc-agent operations-plan --config config.yaml
./bin/btc-agent market-watch --config config.yaml
./bin/btc-agent ops-events --config config.yaml
./bin/btc-agent microstructure-fetch --config config.yaml
./bin/btc-agent status --config config.yaml
./bin/btc-agent run-daily --config config.yaml
./bin/btc-agent paper-manager --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent backtest-live-manager --config config.yaml
./bin/btc-agent real-data-survey --config config.yaml
./bin/btc-agent learn --config config.yaml
./bin/btc-agent universe-research --config config.yaml
./bin/btc-agent live-proof --config config.yaml
./bin/btc-agent live-readiness --config config.yaml
./bin/btc-agent live-auto-audit --config config.yaml
./bin/btc-agent live-doctor --config config.yaml
./bin/btc-agent live-supervisor --config config.yaml --dry-run
./bin/btc-agent reconcile-live-orders --config config.yaml
./bin/btc-agent live-positions --config config.yaml
./bin/btc-agent telegram-commands --config config.yaml
./bin/btc-agent scheduler --config config.yaml --run-now --dry-run
```

Manual proof order là path riêng, cần confirm phrase chính xác và vẫn phải qua live gates:

```bash
./bin/btc-agent execute-live-proof-order --config config.yaml --confirm I_UNDERSTAND_THIS_PLACES_A_REAL_SPOT_LIMIT_ORDER
```

## Live-auto production runtime

Nạp env runtime từ `$HOME/btc-agent.env`:

```bash
set -a; . "$HOME/btc-agent.env"; set +a
export BTC_AGENT_MODE=live-auto
export BTC_AGENT_ALLOW_AUTO_LIVE=true
./bin/btc-agent live-doctor --config config.yaml
./scripts/btc-agent-scheduler.sh
```

Gates live-auto cần bật rõ:

```text
live.enabled=true
live.auto_execute=true
live.require_manual_confirm=false
live.proof_only=false
live.order_management_enabled=true
live.supervisor_enabled=true
execution.real_trading_enabled=true
BTC_AGENT_ALLOW_AUTO_LIVE=true
```

Nếu bất kỳ gate nào fail, bot block hoặc chỉ reconcile.

## Auto-live monitoring

`market-watch` là vòng quét vận hành read-only: fetch dữ liệu mới, fetch microstructure nếu bật, analyze BTC, build Agent2 plan, ghi operations plan, ghi runtime event, và gửi Telegram khi state/critical thay đổi. Nó không đặt/hủy lệnh; live execution vẫn chỉ qua `live-supervisor` và managed order engine.

```bash
./bin/btc-agent market-watch --config config.yaml
./bin/btc-agent operations-plan --config config.yaml
./bin/btc-agent ops-events --config config.yaml
./bin/btc-agent microstructure-fetch --config config.yaml
```

Reports/state:

```text
reports/operations_plan_latest.md/json
reports/market_watch_state.json
SQLite runtime_events
reports/microstructure_latest.md/json
SQLite microstructure_snapshots
```

`operations-plan` hiển thị BTC accumulation phase, quyền live, capital envelope, exposure hiện có, executable budget, opportunity budget, và next trigger. Executable budget chỉ >0 khi đủ `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED`.

`ops-events` đọc pending runtime events từ SQLite để gom tín hiệu vận hành: market state changed, market critical, live supervisor event, microstructure stale/fetch/state event. Lệnh này read-only, không đặt/hủy lệnh.

`microstructure-fetch` là report-only: đọc Binance public spot/futures observation (taker flow/CVD/orderbook/OI/funding/basis), ghi snapshot và report. Dữ liệu futures chỉ quan sát, không futures execution. Nếu `microstructure.require_fresh_for_active=true`, stale/missing microstructure chỉ được dùng để hạ quyền: BTC max `WATCH`, asset không lên `ACTIVE_LIMIT`.

## Pre-live safety hardening

`live-auto-audit` là command kiểm duyệt trước khi cho bot tự gửi lệnh thật. Nó ghi `reports/live_auto_audit_latest.md/json`, chạy forced `ACTIVE_LIMIT` dry-run simulation, kiểm doctor/data/reconcile/risk/microstructure/proof/final assertion, và luôn kết luận rõ:

```text
APPROVED_MONITORING
APPROVED_DRY_RUN
APPROVED_REAL_ORDER
BLOCKED
```

Managed order engine có final execution assertion ngay trước `PlaceSpotLimitOrder`: chặn nếu config live không sạch, risk flags sai, plan không `ACTIVE_LIMIT`, permission không `ALLOWED`, lệnh không phải `BUY limit post-only`, hoặc vượt cap. First-order quarantine mặc định nên bật trước production: chỉ cho 1 layer nhỏ đầu tiên sau dry-run audit, giúp lần live order đầu được kiểm soát.

## Report-only survey và learning

Flow khảo sát dữ liệu thật:

```bash
./bin/btc-agent fetch --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent backtest-live-manager --config config.yaml
./bin/btc-agent real-data-survey --config config.yaml
./bin/btc-agent learn --config config.yaml
```

`real-data-survey` gom các bằng chứng:

- coverage dữ liệu local.
- BTC permission audit.
- BTC accumulation phase forward/false-positive audit.
- Agent2 opportunity/near-miss audit.
- Managed live-manager history simulation.
- Learning actions dạng khuyến nghị thủ công.

Survey/learning chỉ diagnostic. Không sửa `config.yaml`, không đặt/hủy lệnh, không thay đổi quyền live.

Ví dụ kết luận an toàn khi thị trường chưa đủ gate:

```text
plan=SCOUT
BTC permission=WATCH
BTC accumulation=MARKDOWN
desired=0
placed=0
can_submit=false
```

## Reports

File report chính:

```text
reports/latest.md/json
reports/bot_state_latest.md/json
reports/scenario_latest.md/json
reports/filter_attribution_latest.md/json
reports/technical_scorecard_latest.md/json
reports/capital_plan_research_latest.md/json
reports/coin_universe_research_latest.md/json
reports/decision_dashboard_latest.md/json
reports/operations_plan_latest.md/json
reports/auto_live_management_latest.md/json
reports/live_supervisor_latest.md/json
reports/live_doctor_latest.md/json
reports/live_readiness_latest.md/json
reports/live_auto_audit_latest.md/json
reports/live_reconcile_latest.md/json
reports/live_position_latest.md/json
reports/real_data_survey_latest.md/json
reports/learning_latest.md/json
```

Research-only reports không thay production assets, không sửa config, không đặt lệnh.

## Telegram read-only

Allowed commands:

```text
/status
/why
/coins
/filters
/scorecard
/allocation
/capital
/universe
/dashboard
/trigger
/orders
/positions
/doctor
/supervisor
/next
/risk
/help
```

Blocked/not implemented:

```text
/buy /sell /market /leverage /override /resume /halt /cancel /close
```

Telegram chỉ hiển thị state, blockers, dashboard, trigger, orders, positions, doctor, supervisor. Không đặt/hủy/đóng lệnh và không override gates.

## Verification trước khi báo done

Mechanical checks:

```bash
gofmt -w .
go test -v -count=1 ./...
go vet ./...
go build -o bin/btc-agent .
```

Report-only flow:

```bash
./bin/btc-agent fetch --config config.yaml
./bin/btc-agent backtest --config config.yaml
./bin/btc-agent backtest-live-manager --config config.yaml
./bin/btc-agent real-data-survey --config config.yaml
./bin/btc-agent learn --config config.yaml
```

Live dry-run safety:

```bash
set -a; . "$HOME/btc-agent.env"; set +a
BTC_AGENT_MODE=live-auto BTC_AGENT_ALLOW_AUTO_LIVE=true ./bin/btc-agent live-supervisor --config config.yaml --dry-run
./bin/btc-agent live-doctor --config config.yaml
./bin/btc-agent telegram-commands --config config.yaml
```

Expected nếu chưa đủ `ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED`:

```text
desired=0
placed=0
can_submit=false
No real order was placed.
```

## Roadmap

### Milestone A: done

- OHLCV deterministic BTC accumulation detector.
- Agent1 accumulation permission cap.
- Agent2 BTC accumulation gate.
- Backtest accumulation phase forward/false-positive audit.
- Survey/learning/dashboard evidence report-only.

### Milestone B: data sources

- Spot taker buy/sell volume.
- CVD/volume delta.
- Order book imbalance.
- Open interest, funding, liquidation proxy.
- Spot-perp basis.
- Anchored VWAP/volume profile.
- Data stale blockers: stale microstructure => tối đa `WATCH`, không `ACTIVE_LIMIT`.

### Milestone C: proof before sizing

- Walk-forward proof detector giảm false positive/drawdown.
- No live sizing expansion nếu sample thấp hoặc false-positive cao.
- Exit/invalidation engine rõ trước khi tăng quyền live.

## Secrets và vận hành an toàn

- Không paste OKX keys, Telegram token, `.env`, hoặc config thật vào chat/logs.
- Dùng `$HOME/btc-agent.env` cho runtime env.
- Bật OKX IP whitelist và least permissions.
- Rotate mọi key từng bị paste ra ngoài thiết bị.
- `config.yaml`, `.env`, DB, reports, logs, backups, binaries giữ local-only.
