// home: takeover session launcher ("Remagic Home").
// Runs with xochitl stopped. Scans AppLoad bundles, shows a tile grid of
// takeover (qtfb:false) apps, and on tap writes the chosen app dir to
// /tmp/remagic-home-choice and exits 42. The session script then runs that
// app and re-launches home when it exits. "Leave" (or power button /
// 5-finger tap) exits 0, and the session script restores xochitl.
//
// Touch mapping can be flipped via env: HOME_TOUCH_FLIPX=1 HOME_TOUCH_FLIPY=1.

#include <QImage>
#include <QDir>
#include <QFile>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <fcntl.h>
#include <unistd.h>
#include <signal.h>
#include <stdint.h>
#include <sys/ioctl.h>
#include <sys/time.h>
#include <poll.h>
#include <vector>
#include <string>

extern "C" {
int quill_init(void);
int quill_width(void);
int quill_height(void);
int quill_stride(void);
int quill_format(void);
unsigned char *quill_buffer(void);
unsigned long quill_swap(int x, int y, int w, int h, int mode, int full);
void quill_process_events(void);
}

#define EV_KEY 1
#define EV_ABS 3
#define ABS_MT_SLOT 47
#define ABS_MT_POSITION_X 53
#define ABS_MT_POSITION_Y 54
#define ABS_MT_TRACKING_ID 57
#define KEY_POWER 116
#define EVIOCGRAB 0x40044590
#define MAX_SLOTS 16

struct input_event { struct timeval time; uint16_t type; uint16_t code; int32_t value; };
struct input_absinfo { int32_t value, minimum, maximum, fuzz, flat, resolution; };
#define EVIOCGABS(abs) (0x80184540 + (abs))  // _IOR('E', 0x40+abs, struct input_absinfo)

static volatile sig_atomic_t g_quit = 0;
static void on_term(int sig) { (void)sig; g_quit = 1; }

static int W, H, STRIDE, BPP;
static unsigned char *FB;

// ---------------------------------------------------------------- drawing

static void put_gray(int x, int y, unsigned char v) {
    if (x < 0 || y < 0 || x >= W || y >= H) return;
    unsigned char *p = FB + (size_t)y * STRIDE + (size_t)x * BPP;
    memset(p, v, BPP);
    if (BPP == 4) p[3] = 0xFF;
}

static void fill_rect(int x0, int y0, int w, int h, unsigned char v) {
    for (int y = y0; y < y0 + h; y++)
        for (int x = x0; x < x0 + w; x++)
            put_gray(x, y, v);
}

static void rect_outline(int x0, int y0, int w, int h, int t) {
    fill_rect(x0, y0, w, t, 0x00);
    fill_rect(x0, y0 + h - t, w, t, 0x00);
    fill_rect(x0, y0, t, h, 0x00);
    fill_rect(x0 + w - t, y0, t, h, 0x00);
}

// Classic 5x7 bitmap font, ASCII 32..126. Column-major, LSB = top row.
static const unsigned char FONT5X7[95][5] = {
    {0x00,0x00,0x00,0x00,0x00},{0x00,0x00,0x5F,0x00,0x00},{0x00,0x07,0x00,0x07,0x00},
    {0x14,0x7F,0x14,0x7F,0x14},{0x24,0x2A,0x7F,0x2A,0x12},{0x23,0x13,0x08,0x64,0x62},
    {0x36,0x49,0x55,0x22,0x50},{0x00,0x05,0x03,0x00,0x00},{0x00,0x1C,0x22,0x41,0x00},
    {0x00,0x41,0x22,0x1C,0x00},{0x14,0x08,0x3E,0x08,0x14},{0x08,0x08,0x3E,0x08,0x08},
    {0x00,0x50,0x30,0x00,0x00},{0x08,0x08,0x08,0x08,0x08},{0x00,0x60,0x60,0x00,0x00},
    {0x20,0x10,0x08,0x04,0x02},{0x3E,0x51,0x49,0x45,0x3E},{0x00,0x42,0x7F,0x40,0x00},
    {0x42,0x61,0x51,0x49,0x46},{0x21,0x41,0x45,0x4B,0x31},{0x18,0x14,0x12,0x7F,0x10},
    {0x27,0x45,0x45,0x45,0x39},{0x3C,0x4A,0x49,0x49,0x30},{0x01,0x71,0x09,0x05,0x03},
    {0x36,0x49,0x49,0x49,0x36},{0x06,0x49,0x49,0x29,0x1E},{0x00,0x36,0x36,0x00,0x00},
    {0x00,0x56,0x36,0x00,0x00},{0x08,0x14,0x22,0x41,0x00},{0x14,0x14,0x14,0x14,0x14},
    {0x00,0x41,0x22,0x14,0x08},{0x02,0x01,0x51,0x09,0x06},{0x32,0x49,0x79,0x41,0x3E},
    {0x7E,0x11,0x11,0x11,0x7E},{0x7F,0x49,0x49,0x49,0x36},{0x3E,0x41,0x41,0x41,0x22},
    {0x7F,0x41,0x41,0x22,0x1C},{0x7F,0x49,0x49,0x49,0x41},{0x7F,0x09,0x09,0x09,0x01},
    {0x3E,0x41,0x49,0x49,0x7A},{0x7F,0x08,0x08,0x08,0x7F},{0x00,0x41,0x7F,0x41,0x00},
    {0x20,0x40,0x41,0x3F,0x01},{0x7F,0x08,0x14,0x22,0x41},{0x7F,0x40,0x40,0x40,0x40},
    {0x7F,0x02,0x0C,0x02,0x7F},{0x7F,0x04,0x08,0x10,0x7F},{0x3E,0x41,0x41,0x41,0x3E},
    {0x7F,0x09,0x09,0x09,0x06},{0x3E,0x41,0x51,0x21,0x5E},{0x7F,0x09,0x19,0x29,0x46},
    {0x46,0x49,0x49,0x49,0x31},{0x01,0x01,0x7F,0x01,0x01},{0x3F,0x40,0x40,0x40,0x3F},
    {0x1F,0x20,0x40,0x20,0x1F},{0x3F,0x40,0x38,0x40,0x3F},{0x63,0x14,0x08,0x14,0x63},
    {0x07,0x08,0x70,0x08,0x07},{0x61,0x51,0x49,0x45,0x43},{0x00,0x7F,0x41,0x41,0x00},
    {0x02,0x04,0x08,0x10,0x20},{0x00,0x41,0x41,0x7F,0x00},{0x04,0x02,0x01,0x02,0x04},
    {0x40,0x40,0x40,0x40,0x40},{0x00,0x01,0x02,0x04,0x00},{0x20,0x54,0x54,0x54,0x78},
    {0x7F,0x48,0x44,0x44,0x38},{0x38,0x44,0x44,0x44,0x20},{0x38,0x44,0x44,0x48,0x7F},
    {0x38,0x54,0x54,0x54,0x18},{0x08,0x7E,0x09,0x01,0x02},{0x0C,0x52,0x52,0x52,0x3E},
    {0x7F,0x08,0x04,0x04,0x78},{0x00,0x44,0x7D,0x40,0x00},{0x20,0x40,0x44,0x3D,0x00},
    {0x7F,0x10,0x28,0x44,0x00},{0x00,0x41,0x7F,0x40,0x00},{0x7C,0x04,0x18,0x04,0x78},
    {0x7C,0x08,0x04,0x04,0x78},{0x38,0x44,0x44,0x44,0x38},{0x7C,0x14,0x14,0x14,0x08},
    {0x08,0x14,0x14,0x18,0x7C},{0x7C,0x08,0x04,0x04,0x08},{0x48,0x54,0x54,0x54,0x20},
    {0x04,0x3F,0x44,0x40,0x20},{0x3C,0x40,0x40,0x20,0x7C},{0x1C,0x20,0x40,0x20,0x1C},
    {0x3C,0x40,0x30,0x40,0x3C},{0x44,0x28,0x10,0x28,0x44},{0x0C,0x50,0x50,0x50,0x3C},
    {0x44,0x64,0x54,0x4C,0x44},{0x00,0x08,0x36,0x41,0x00},{0x00,0x00,0x7F,0x00,0x00},
    {0x00,0x41,0x36,0x08,0x00},{0x08,0x08,0x2A,0x1C,0x08},
};

static void draw_text(int x, int y, const char *s, int scale) {
    for (; *s; s++) {
        unsigned char c = (unsigned char)*s;
        if (c < 32 || c > 126) c = '?';
        const unsigned char *g = FONT5X7[c - 32];
        for (int col = 0; col < 5; col++)
            for (int row = 0; row < 7; row++)
                if (g[col] & (1 << row))
                    fill_rect(x + col * scale, y + row * scale, scale, scale, 0x00);
        x += 6 * scale;
    }
}

static int text_width(const char *s, int scale) { return (int)strlen(s) * 6 * scale; }

// Bayer ordered dither, same as image_demo.
static unsigned char ordered_bw(int luma, int x, int y) {
    static const int bayer8[8][8] = {
        { 0,48,12,60, 3,51,15,63}, {32,16,44,28,35,19,47,31},
        { 8,56, 4,52,11,59, 7,55}, {40,24,36,20,43,27,39,23},
        { 2,50,14,62, 1,49,13,61}, {34,18,46,30,33,17,45,29},
        {10,58, 6,54, 9,57, 5,53}, {42,26,38,22,41,25,37,21},
    };
    int threshold = (bayer8[y & 7][x & 7] * 255 + 31) / 63;
    return luma < threshold ? 0x00 : 0xFF;
}

static void blit_icon(const QImage &src, int dx, int dy, int size) {
    QImage s = src.convertToFormat(QImage::Format_RGB32)
        .scaled(size, size, Qt::KeepAspectRatio, Qt::SmoothTransformation);
    int ox = dx + (size - s.width()) / 2, oy = dy + (size - s.height()) / 2;
    for (int y = 0; y < s.height(); y++) {
        const QRgb *row = (const QRgb *)s.constScanLine(y);
        for (int x = 0; x < s.width(); x++) {
            int l = (qRed(row[x]) * 30 + qGreen(row[x]) * 59 + qBlue(row[x]) * 11) / 100;
            put_gray(ox + x, oy + y, ordered_bw(l, ox + x, oy + y));
        }
    }
}

// ---------------------------------------------------------------- apps

struct App { std::string dir, name, script; };

// Minimal JSON field extraction — manifests are flat and machine-written.
static std::string json_str(const std::string &j, const char *key) {
    std::string pat = std::string("\"") + key + "\"";
    size_t k = j.find(pat); if (k == std::string::npos) return "";
    size_t c = j.find(':', k + pat.size()); if (c == std::string::npos) return "";
    size_t q1 = j.find('"', c + 1); if (q1 == std::string::npos) return "";
    size_t q2 = j.find('"', q1 + 1); if (q2 == std::string::npos) return "";
    return j.substr(q1 + 1, q2 - q1 - 1);
}

static bool json_bool(const std::string &j, const char *key, bool dflt) {
    std::string pat = std::string("\"") + key + "\"";
    size_t k = j.find(pat); if (k == std::string::npos) return dflt;
    size_t c = j.find(':', k + pat.size()); if (c == std::string::npos) return dflt;
    return j.find("true", c) == j.find_first_not_of(" \t", c + 1);
}

static std::vector<App> scan_apps(const char *root) {
    std::vector<App> out;
    QDir d(root);
    for (const QString &e : d.entryList(QDir::Dirs | QDir::NoDotAndDotDot, QDir::Name)) {
        std::string dir = std::string(root) + "/" + e.toStdString();
        QFile mf(QString::fromStdString(dir + "/external.manifest.json"));
        if (!mf.open(QIODevice::ReadOnly)) continue;
        std::string j = mf.readAll().toStdString();
        if (json_bool(j, "qtfb", true)) continue;          // qtfb apps need xochitl
        if (json_str(j, "id") == "home") continue;         // don't list ourselves
        // Session-launchable convention: a *-takeover.sh in the bundle.
        QDir ad(QString::fromStdString(dir));
        QString script;
        for (const QString &f : ad.entryList(QStringList() << "*-takeover.sh", QDir::Files))
            { script = f; break; }
        if (script.isEmpty()) continue;
        std::string name = json_str(j, "name");
        if (name.empty()) name = e.toStdString();
        out.push_back({dir, name, dir + "/" + script.toStdString()});
    }
    return out;
}

// ---------------------------------------------------------------- input

static int open_input(const char *needle, char *out_path) {
    char path[64], name[128], lower[128];
    for (int i = 0; i < 8; i++) {
        snprintf(path, sizeof path, "/sys/class/input/event%d/device/name", i);
        FILE *f = fopen(path, "r"); if (!f) continue;
        if (!fgets(name, sizeof name, f)) { fclose(f); continue; }
        fclose(f); memset(lower, 0, sizeof lower);
        for (size_t j = 0; j < sizeof lower - 1 && name[j]; j++)
            lower[j] = (name[j] >= 'A' && name[j] <= 'Z') ? name[j] + 32 : name[j];
        if (!strstr(lower, needle)) continue;
        snprintf(path, sizeof path, "/dev/input/event%d", i);
        int fd = open(path, O_RDONLY | O_NONBLOCK);
        if (fd >= 0) { int one = 1; ioctl(fd, EVIOCGRAB, &one); if (out_path) strcpy(out_path, path); }
        return fd;
    }
    return -1;
}

int main(void) {
    signal(SIGTERM, on_term); signal(SIGINT, on_term);
    if (quill_init() != 0) return 1;
    W = quill_width(); H = quill_height(); STRIDE = quill_stride();
    BPP = STRIDE / (W ? W : 1); FB = quill_buffer();
    if (!FB || W <= 0) return 1;

    const char *root = getenv("REMAGIC_APPS");
    if (!root) root = "/home/root/xovi/exthome/appload";
    std::vector<App> apps = scan_apps(root);
    fprintf(stderr, "home: %zu takeover apps under %s\n", apps.size(), root);

    // ---- layout: 2-column tile grid + Leave button at the bottom.
    const int COLS = 2, PAD = 60;
    int tile = (W - PAD * (COLS + 1)) / COLS;
    int th = tile * 2 / 3;
    int header = 220;
    int leave_h = 140, leave_w = 520;
    int leave_x = (W - leave_w) / 2, leave_y = H - leave_h - 100;

    memset(FB, 0xFF, (size_t)STRIDE * H);
    {
        const char *title = "REMAGIC";
        draw_text((W - text_width(title, 8)) / 2, 70, title, 8);
    }
    struct Tile { int x, y, w, h; };
    std::vector<Tile> tiles;
    auto render_tile = [&](size_t i) {
        const Tile &t = tiles[i];
        fill_rect(t.x, t.y, t.w, t.h, 0xFF);
        rect_outline(t.x, t.y, t.w, t.h, 4);
        QImage icon(QString::fromStdString(apps[i].dir + "/icon.png"));
        int isz = t.h - 120;
        if (!icon.isNull()) blit_icon(icon, t.x + (t.w - isz) / 2, t.y + 30, isz);
        std::string nm = apps[i].name.substr(0, 18);
        draw_text(t.x + (t.w - text_width(nm.c_str(), 4)) / 2, t.y + t.h - 70, nm.c_str(), 4);
    };
    for (size_t i = 0; i < apps.size(); i++) {
        int cx = PAD + (int)(i % COLS) * (tile + PAD);
        int cy = header + (int)(i / COLS) * (th + PAD);
        if (cy + th > leave_y - PAD) break; // one page for now
        tiles.push_back({cx, cy, tile, th});
        render_tile(i);
    }
    if (apps.empty()) {
        const char *msg = "No takeover apps found";
        draw_text((W - text_width(msg, 4)) / 2, H / 2, msg, 4);
    }
    rect_outline(leave_x, leave_y, leave_w, leave_h, 4);
    draw_text(leave_x + (leave_w - text_width("LEAVE", 5)) / 2,
              leave_y + (leave_h - 35) / 2, "LEAVE", 5);
    quill_swap(0, 0, W, H, 3, 1);

    // ---- input loop: tap detection on finger release.
    char tpath[64] = "";
    int pwr_fd = open_input("powerkey", NULL);
    int touch_fd = open_input("touch", tpath);
    int tmaxx = 0, tmaxy = 0;
    if (touch_fd >= 0) {
        struct input_absinfo ai;
        if (ioctl(touch_fd, EVIOCGABS(ABS_MT_POSITION_X), &ai) == 0) tmaxx = ai.maximum;
        if (ioctl(touch_fd, EVIOCGABS(ABS_MT_POSITION_Y), &ai) == 0) tmaxy = ai.maximum;
    }
    if (tmaxx <= 0) tmaxx = W - 1;
    if (tmaxy <= 0) tmaxy = H - 1;
    int flipx = getenv("HOME_TOUCH_FLIPX") != NULL;
    int flipy = getenv("HOME_TOUCH_FLIPY") != NULL;
    fprintf(stderr, "home: touch %s max %dx%d flip %d%d\n", tpath, tmaxx, tmaxy, flipx, flipy);

    struct pollfd pfds[2] = {{.fd = pwr_fd, .events = POLLIN}, {.fd = touch_fd, .events = POLLIN}};
    int slot_active[MAX_SLOTS] = {0}, cur_slot = 0;
    int tx = -1, ty = -1, down = 0, chosen = -1, leave = 0;
    int pressed = -2;   // -2 none, -1 leave button, >=0 tile index (visual press state)

    // Hit test in screen coords: -1 leave, >=0 tile, -2 nothing.
    auto hit = [&](int px, int py) -> int {
        if (px >= leave_x && px < leave_x + leave_w &&
            py >= leave_y && py < leave_y + leave_h) return -1;
        for (size_t t = 0; t < tiles.size(); t++)
            if (px >= tiles[t].x && px < tiles[t].x + tiles[t].w &&
                py >= tiles[t].y && py < tiles[t].y + tiles[t].h) return (int)t;
        return -2;
    };

    while (!g_quit && chosen < 0 && !leave) {
        poll(pfds, 2, 20);
        struct input_event evs[64]; ssize_t n;
        int released = 0;
        if (pwr_fd >= 0) while ((n = read(pwr_fd, evs, sizeof evs)) > 0)
            for (int i = 0; i < (int)(n / sizeof evs[0]); i++)
                if (evs[i].type == EV_KEY && evs[i].code == KEY_POWER && evs[i].value == 1) leave = 1;
        if (touch_fd >= 0) while ((n = read(touch_fd, evs, sizeof evs)) > 0)
            for (int i = 0; i < (int)(n / sizeof evs[0]); i++) {
                struct input_event *e = &evs[i];
                if (e->type == EV_ABS && e->code == ABS_MT_SLOT)
                    cur_slot = e->value < 0 ? 0 : (e->value >= MAX_SLOTS ? MAX_SLOTS - 1 : e->value);
                else if (e->type == EV_ABS && e->code == ABS_MT_POSITION_X && cur_slot == 0)
                    tx = (int)((int64_t)e->value * (W - 1) / tmaxx);
                else if (e->type == EV_ABS && e->code == ABS_MT_POSITION_Y && cur_slot == 0)
                    ty = (int)((int64_t)e->value * (H - 1) / tmaxy);
                else if (e->type == EV_ABS && e->code == ABS_MT_TRACKING_ID) {
                    slot_active[cur_slot] = e->value != -1;
                    int fingers = 0; for (int s = 0; s < MAX_SLOTS; s++) fingers += slot_active[s];
                    if (fingers >= 5) leave = 1;
                    if (cur_slot == 0) {
                        if (e->value != -1) down = 1;
                        else if (down) { down = 0; released = 1; }
                    }
                }
            }

        int px = flipx ? W - 1 - tx : tx;
        int py = flipy ? H - 1 - ty : ty;

        // Instant press feedback: invert the target the moment the finger is
        // down on it (coords can arrive an event-frame after the touch-down,
        // so this runs every loop while down). Fast DU swap = ~instant.
        if (down && pressed == -2 && tx >= 0) {
            int h = hit(px, py);
            if (h == -1) {
                fill_rect(leave_x, leave_y, leave_w, leave_h, 0x00);
                quill_swap(leave_x, leave_y, leave_w, leave_h, 0, 0);
                pressed = -1;
            } else if (h >= 0) {
                fill_rect(tiles[h].x, tiles[h].y, tiles[h].w, tiles[h].h, 0x00);
                quill_swap(tiles[h].x, tiles[h].y, tiles[h].w, tiles[h].h, 0, 0);
                pressed = h;
            }
        }

        if (released && tx >= 0) {
            int h = hit(px, py);
            if (h == -1 && pressed == -1) leave = 1;
            else if (h >= 0 && h == pressed) chosen = h;
            else if (pressed != -2) {
                // Finger slid off: cancel — restore the pressed target.
                if (pressed == -1) {
                    fill_rect(leave_x, leave_y, leave_w, leave_h, 0xFF);
                    rect_outline(leave_x, leave_y, leave_w, leave_h, 4);
                    draw_text(leave_x + (leave_w - text_width("LEAVE", 5)) / 2,
                              leave_y + (leave_h - 35) / 2, "LEAVE", 5);
                    quill_swap(leave_x, leave_y, leave_w, leave_h, 0, 0);
                } else {
                    render_tile(pressed);
                    quill_swap(tiles[pressed].x, tiles[pressed].y, tiles[pressed].w, tiles[pressed].h, 0, 0);
                }
            }
            pressed = -2;
        }
        quill_process_events();
    }

    if (chosen >= 0) {
        // Tile is already inverted from press feedback; add a launch note so
        // the engine-init gap (a few seconds) doesn't read as a hang.
        {
            std::string msg = "STARTING " + apps[chosen].name.substr(0, 14);
            for (auto &c : msg) c = (c >= 'a' && c <= 'z') ? c - 32 : c;
            fill_rect(0, leave_y - 90, W, 60, 0xFF);
            draw_text((W - text_width(msg.c_str(), 4)) / 2, leave_y - 90, msg.c_str(), 4);
            quill_swap(0, leave_y - 90, W, 60, 0, 0);
        }
        FILE *f = fopen("/tmp/remagic-home-choice", "w");
        if (f) { fprintf(f, "%s\n", apps[chosen].script.c_str()); fclose(f); }
        fprintf(stderr, "home: launching %s\n", apps[chosen].script.c_str());
        return 42;
    }
    // Leaving: xochitl takes several seconds to boot — say so, or the frozen
    // grid reads as a hang.
    memset(FB, 0xFF, (size_t)STRIDE * H);
    {
        const char *m1 = "LEAVING...";
        const char *m2 = "RESTORING YOUR TABLET";
        draw_text((W - text_width(m1, 6)) / 2, H / 2 - 80, m1, 6);
        draw_text((W - text_width(m2, 3)) / 2, H / 2 + 20, m2, 3);
    }
    quill_swap(0, 0, W, H, 0, 0);
    fprintf(stderr, "home: leave\n");
    return 0;
}
