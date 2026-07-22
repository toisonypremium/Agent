#!/usr/bin/env python3
"""
Tính và cập nhật phân bổ vốn động theo vốn thực tế trong tài khoản.
Bot dùng layer_distribution để chia vốn vào 3 mức giá khác nhau (Support High/Mid/Low),
không all-in mà trải đều theo tỷ lệ tăng dần để gom đẹp hơn khi giá giảm.

Logic hoàn chỉnh:
- Mỗi đồng coin có budget riêng: total_capital × allocation% × max_deploy%
- Budget mỗi đồng chia theo layer_distribution (VD: 25% / 35% / 40% cho L1/L2/L3)
- L1 đặt tại Support.High (giá cao hơn), ít vốn hơn
- L2 đặt tại Support.Mid (giữa), vốn trung bình
- L3 đặt tại Support.Low (giá thấp nhất), nhiều vốn nhất → gom nhiều nhất khi giá rẻ nhất
"""
import yaml, math, re

CONFIG = '/home/admin/btc-agent/btc-agent/config.yaml'

with open(CONFIG, 'r') as f:
    raw = f.read()

cfg = yaml.safe_load(raw)

total_capital  = cfg['portfolio']['total_capital']
allocation     = cfg['portfolio']['allocation']
max_deploy     = cfg['risk']['max_total_deployment_per_cycle']
max_single     = cfg['risk']['max_single_asset_deployment']
layer_dist     = cfg['execution']['layer_distribution']
max_layers     = len(layer_dist)
n_assets       = len(allocation)

print("=== Dynamic Capital Allocation (Layer-Aware MM) ===")
print(f"Total capital       : {total_capital} USDT")
print(f"Max deploy/cycle    : {max_deploy*100:.0f}%")
print(f"Max single asset    : {max_single*100:.0f}%")
print(f"Layer distribution  : {[f'{x*100:.0f}%' for x in layer_dist]} (L1→L2→L3)")
print()
print("Chiến lược: L1 (giá cao, ít vốn) → L2 (giữa) → L3 (giá thấp nhất, nhiều vốn nhất)")
print("Bot KHÔNG all-in. Gom theo từng giai đoạn khi giá giảm về vùng support.")
print()

per_asset = {}
for symbol, alloc_pct in allocation.items():
    budget = total_capital * alloc_pct * max_deploy
    budget = min(budget, total_capital * max_single)
    layers_usdt = [budget * f for f in layer_dist]
    per_order_max = max(layers_usdt)  # lệnh lớn nhất (L3)
    per_asset[symbol] = {
        'alloc': alloc_pct,
        'budget': budget,
        'layers': layers_usdt,
        'per_order_max': per_order_max
    }
    print(f"  {symbol} ({alloc_pct*100:.0f}%):")
    print(f"    Budget tổng    : {budget:.1f} USDT")
    for i, (usdt, pct) in enumerate(zip(layers_usdt, layer_dist), 1):
        print(f"    Layer {i} ({pct*100:.0f}%)   : {usdt:.1f} USDT ← Limit Buy tại Support {'High' if i==1 else 'Mid' if i==2 else 'Low'}")
    print()

# max_live_notional_per_order = lệnh L3 lớn nhất (SOL L3)
new_per_order = math.ceil(max(v['per_order_max'] for v in per_asset.values()) / 10) * 10
# max_live_notional_per_asset = budget lớn nhất (SOL tổng 3 layers)
new_per_asset = math.ceil(max(v['budget'] for v in per_asset.values()) / 10) * 10
# total = tổng tất cả budget
new_total     = math.ceil(sum(v['budget'] for v in per_asset.values()) / 10) * 10

# Layer distribution giữ nguyên - bot tự tính notional/layer theo fraction
# Đảm bảo max_auto_layers_per_asset = số layers trong distribution
new_max_layers = len(layer_dist)

print(f"=== Config Values Tối ưu ===")
print(f"  max_live_notional_per_order_usdt : {new_per_order}  (L3 lệnh lớn nhất ~ SOL)")
print(f"  max_live_notional_per_asset_usdt : {new_per_asset}  (tổng 3 layers ~ SOL budget)")
print(f"  max_live_notional_total_usdt     : {new_total}  (tổng tất cả coin cùng lúc)")
print(f"  max_auto_layers_per_asset        : {new_max_layers}")
print()

# Cập nhật config
replacements = [
    (r'  max_live_notional_per_order_usdt: [\d.]+',  f'  max_live_notional_per_order_usdt: {new_per_order}'),
    (r'  max_live_notional_per_asset_usdt: [\d.]+',  f'  max_live_notional_per_asset_usdt: {new_per_asset}'),
    (r'  max_live_notional_total_usdt: [\d.]+',      f'  max_live_notional_total_usdt: {new_total}'),
    (r'  max_order_notional_usdt: [\d]+',            f'  max_order_notional_usdt: {new_per_order}'),
    (r'  live_auto_max_notional_usdt: [\d.]+',       f'  live_auto_max_notional_usdt: {new_per_order}'),
    (r'  first_order_max_notional_usdt: [\d.]+',     f'  first_order_max_notional_usdt: {new_per_order}'),
    (r'  max_auto_layers_per_asset: [\d]+',          f'  max_auto_layers_per_asset: {new_max_layers}'),
]

updated = raw
for pattern, replacement in replacements:
    new_content = re.sub(pattern, replacement, updated)
    changed = new_content != updated
    updated = new_content
    val = replacement.split(': ', 1)[1] if ': ' in replacement else replacement
    print(f"  {'[OK]' if changed else '[--]'} {pattern.split(':')[0].strip().replace('  ', '')} = {val}")

with open(CONFIG, 'w') as f:
    f.write(updated)

# Verify
print("\n=== Xác minh config cuối cùng ===")
with open(CONFIG) as f:
    for line in f:
        if any(k in line for k in ['max_live_notional', 'max_order_notional',
                                    'live_auto_max', 'first_order_max',
                                    'max_auto_layers', 'layer_distribution']):
            print(f"  {line.rstrip()}")

print("\nDONE. Bot sẽ tự tính lại mỗi khi portfolio.total_capital thay đổi.")
