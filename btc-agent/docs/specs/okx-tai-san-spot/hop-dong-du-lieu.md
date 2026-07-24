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
      "trang_thai_gan_thesis": "khong_ap_dung",
      "gia_usdt": "1",
      "gia_tri_usdt": "135.42",
      "trang_thai_dinh_gia": "dinh_gia"
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
- `USDT` được định giá cố định 1 USDT.
- Coin khác chỉ được định giá bằng ticker Spot công khai `*-USDT` của OKX cùng lần quan sát.
- Coin thiếu ticker hợp lệ giữ `chua_dinh_gia`; không gán giá trị 0 và không cộng vào tổng/tỷ trọng.
- `trang_thai_gan_thesis` chỉ là quan sát đối soát; không tạo hoặc sửa thesis.
