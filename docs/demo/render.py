"""Render demo.html to PNG frames by stepping its deterministic timeline.

The README animation (demo.gif) is built from these frames:

    pip install playwright          # drives an installed Microsoft Edge
    python render.py 15             # writes frames/f0000.png ... at 15 fps
    ffmpeg -framerate 15 -i frames/f%04d.png -vf "split[a][b];[a]palettegen=max_colors=64:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" -loop 0 demo.gif

Open demo.html in a browser to watch it live.
"""
import pathlib
import sys

from playwright.sync_api import sync_playwright

HERE = pathlib.Path(__file__).parent
FPS = int(sys.argv[1]) if len(sys.argv) > 1 else 15
ONLY = [int(x) for x in sys.argv[2].split(",")] if len(sys.argv) > 2 else None
out = HERE / ("preview" if ONLY else "frames")
out.mkdir(exist_ok=True)
for f in out.glob("*.png"):
    f.unlink()

with sync_playwright() as p:
    browser = p.chromium.launch(channel="msedge")
    page = browser.new_page(viewport={"width": 820, "height": 460}, device_scale_factor=1)
    page.goto((HERE / "demo.html").as_uri() + "?manual=1")
    page.evaluate("document.fonts.ready")
    page.wait_for_timeout(800)
    total = page.evaluate("window.TOTAL_MS")
    if ONLY:
        for ms in ONLY:
            page.evaluate(f"window.renderAt({ms})")
            page.screenshot(path=str(out / f"t{ms:05d}.png"))
    else:
        n = int(total / 1000 * FPS)
        for i in range(n):
            page.evaluate(f"window.renderAt({i * 1000 / FPS})")
            page.screenshot(path=str(out / f"f{i:04d}.png"))
        print("frames", n, "total_ms", total)
    fonts = page.evaluate("[...document.fonts].filter(f => f.status === 'loaded').map(f => f.family + ' ' + f.weight)")
    print("fonts", sorted(set(fonts)))
    browser.close()
