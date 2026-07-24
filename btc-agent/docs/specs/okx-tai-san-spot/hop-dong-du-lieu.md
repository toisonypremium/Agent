# Hợp đồng dữ liệu: Tài sản Spot OKX

## Artifact cố định

```text
runtime/observer/web_console_okx_assets.json
```

Web Console chỉ được đọc filename này qua allowlist. Không directory listing,
không glob, không đường dẫn từ query string.

## JSON schema logic

```json
{
  "schema_version": 1,
  "nguon": "okx_spot_read_only",
  "thoi_diem_quan_sat": "2026-07-24T03:20:00Z",
  "trang_thai": "da_xac_minh",
  "tai_san": [
    {
      "ma_tai_san": "USDT",
      "kha_dung": "125.42",
      "dang_khoa": "10.00",
      "tong": "135.42",
      "trang_thai_gan_thesis": "khong_ap_dung"
    }
  ],
  "canh_bao": []
}
```

## Trạng thái artifact

```text
da_xac_minh
khong_kha_dung
du_lieu_cu
du_lieu_bat_thuong
```

## Bất biến dữ liệu

- `tong = kha_dung + dang_khoa` theo decimal chính xác; không float binary.
- Mọi giá trị số phải không âm và finite.
- `USDT` được hiển thị riêng, không tự tổng hợp giá trị coin khác.
- Giá trị quy đổi USDT cho coin khác chỉ xuất hiện khi có nguồn giá đã allowlist,
  timestamp tương thích và provenance rõ.
- `trang_thai_gan_thesis` chỉ là quan sát đối soát; không tạo hoặc sửa thesis.
