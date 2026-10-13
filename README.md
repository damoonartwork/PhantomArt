# PhantomArt

یک کلاینت فیلترشکن دسکتاپ (فعلاً ویندوز) بر پایه‌ی [Xray-core](https://github.com/XTLS/Xray-core).
کد با Go و [Wails](https://wails.io) نوشته شده تا بعداً بتوان آن را به مک، لینوکس و (با gomobile) اندروید هم برد.

## امکانات

- پروتکل‌ها: **VLESS** (همراه با Reality و XTLS-Vision)، **VMess**، **Trojan**، **Shadowsocks**
- ترنسپورت‌ها: TCP، WebSocket، gRPC، XHTTP، HTTPUpgrade، HTTP/2، mKCP
- وارد کردن لینک کانفیگ از کلیپ‌بورد یا به‌صورت دستی
- **سابسکریپشن** (base64 یا متن ساده) — وقتی متصل هستید، به‌روزرسانی از داخل تونل انجام می‌شود
- تنظیم خودکار **پروکسی سیستم ویندوز** و برگرداندن آن هنگام قطع/بستن برنامه
- **مسیریابی هوشمند**: سایت‌ها و IPهای ایرانی مستقیم باز می‌شوند (`geosite:category-ir`، `geoip:ir`)
- **فرگمنت (Fragment)**: تکه‌تکه کردن TLS ClientHello برای دور زدن فیلتر SNI
- مسدود کردن تبلیغات (اختیاری)
- پینگ TCP همه سرورها و تست واقعی اتصال از داخل تونل
- رابط کاربری فارسی (راست‌به‌چپ)
- پروسه‌ی Xray به برنامه گره خورده؛ اگر برنامه کرش کند، Xray هم بسته می‌شود

## سرور

برنامه به یک سرور خارج از کشور نیاز دارد. راهنمای قدم‌به‌قدم (فارسی، بدون نیاز به برنامه‌نویسی) در
[`server/README.md`](server/README.md) است:

- **رایگان:** سرور روی Cloudflare Workers ([`server/cloudflare-worker/worker.js`](server/cloudflare-worker/worker.js))
- **VPS:** نصب VLESS + Reality با یک دستور ([`server/vps/install.sh`](server/vps/install.sh))

## ساختار پروژه

```
main.go, app.go           برنامه‌ی Wails و متدهایی که رابط کاربری صدا می‌زند
frontend/dist/            رابط کاربری (HTML/CSS/JS ساده، بدون نیاز به npm)
internal/link/            خواندن لینک‌های vless:// vmess:// trojan:// ss://
internal/xray/            ساخت کانفیگ JSON برای Xray
internal/core/            اجرا و مدیریت پروسه‌ی xray.exe
internal/sysproxy/        تنظیم پروکسی سیستم (ویندوز)
internal/store/           ذخیره‌ی سرورها و تنظیمات در %APPDATA%\PhantomArt
internal/subscription/    دانلود سابسکریپشن، پینگ و تست اتصال
server/                   سرور: Cloudflare Worker و اسکریپت نصب VPS
```

## ساخت (Build) روی ویندوز

پیش‌نیاز: [Go 1.24+](https://go.dev/dl/). ویندوز ۱۰/۱۱ خودش WebView2 را دارد.

```powershell
.\scripts\build.ps1
```

خروجی در `build\PhantomArt\` قرار می‌گیرد:

```
PhantomArt.exe
core\xray.exe
core\geoip.dat
core\geosite.dat
```

اسکریپت آخرین نسخه‌ی Xray-core را خودش دانلود می‌کند. اگر GitHub در دسترس نیست،
فایل `Xray-windows-64.zip` را دستی از
[صفحه‌ی ریلیزهای Xray](https://github.com/XTLS/Xray-core/releases) بگیرید و محتوایش را در پوشه‌ی `core` بریزید.

اگر [Wails CLI](https://wails.io/docs/gettingstarted/installation) را نصب کرده‌اید، `wails dev` هم برای توسعه کار می‌کند.

هر push به GitHub هم با GitHub Actions یک نسخه‌ی آماده‌ی ویندوز (به‌همراه Xray) می‌سازد که از تب **Actions** قابل دانلود است.

## استفاده

1. لینک کانفیگ (مثلاً `vless://...`) را کپی کنید و «افزودن از کلیپ‌بورد» را بزنید، یا لینک سابسکریپشن را در تب «سابسکریپشن» وارد کنید.
2. سرور را انتخاب کنید (دوبار کلیک = اتصال مستقیم).
3. دکمه‌ی پاور را بزنید. با «تست اتصال» مطمئن شوید تونل کار می‌کند.

پروکسی‌های محلی: SOCKS روی `127.0.0.1:10808` و HTTP روی `127.0.0.1:10809` (قابل تغییر در تنظیمات).

## تست‌ها

```bash
go test ./internal/...
# تست‌های یکپارچه با Xray واقعی:
PHANTOMART_XRAY_DIR=/path/to/xray-dir go test ./internal/core
```

## نقشه‌ی راه

- [ ] حالت TUN (تونل کردن کل ترافیک سیستم، نه فقط برنامه‌هایی که پروکسی سیستم را رعایت می‌کنند)
- [ ] آیکون در System Tray و اجرای خودکار با ویندوز
- [ ] اسکن QR کد
- [ ] پشتیبانی از Hysteria2 / TUIC (با sing-box)
- [ ] نسخه‌ی مک و لینوکس (Wails) و اندروید (gomobile + VpnService)
