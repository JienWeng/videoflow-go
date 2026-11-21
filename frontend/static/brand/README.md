# VideoFlow symbol

The mark is an open film gate made from two interlocking ribbons. It contains no lettering, fonts or stock icon. The canonical geometry is `videoflow-symbol.svg`: ivory `#F5EFE7` and coral `#F47D5B`. On light surfaces the ivory ribbon becomes ink `#19201F`. The inline BrandLogo component adapts that one source to the app theme.

`videoflow-mark.svg` adds the dark `#111615` app-icon surface. PNG browser/touch icons and desktop PNG/ICO/ICNS assets are deterministic rasterizations of that geometry. `videoflow-wordmark.svg` is a legacy URL alias containing the new symbol, so older references no longer show the retired lettering.

To regenerate with an installed Playwright/Chromium environment:

    PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs CHROME_BIN=/absolute/path/to/chrome node scripts/generate_brand_icons.mjs

The script writes web assets here and native assets under `electron/assets` and `electron/build`. It makes a dark/light small-size review sheet at `/tmp/videoflow-brand-review.png`. It does not call an image-generation API. Keep visible usage square; do not stretch the mark into the old wordmark proportions. Preserve accessible product-name labels on home links and document/app titles.

Desktop packaging uses the new sources on its next build. Existing published installer binaries are not rewritten by updating these files.
