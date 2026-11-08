#include <stdio.h>
#include <string.h>
#include <time.h>
#include <wayland-client.h>
#include "vp.h"
static struct zwlr_virtual_pointer_manager_v1 *mgr;
static void g(void *d, struct wl_registry *r, uint32_t n, const char *i, uint32_t v) {
  if (!strcmp(i, zwlr_virtual_pointer_manager_v1_interface.name)) mgr = wl_registry_bind(r, n, &zwlr_virtual_pointer_manager_v1_interface, 1);
}
static void gr(void *d, struct wl_registry *r, uint32_t n) {}
static const struct wl_registry_listener L = { g, gr };
static uint32_t now(void) { struct timespec t; clock_gettime(CLOCK_MONOTONIC, &t); return t.tv_sec * 1000 + t.tv_nsec / 1000000; }
static void btn(struct zwlr_virtual_pointer_v1 *p, uint32_t b, int down) { zwlr_virtual_pointer_v1_button(p, now(), b, down); zwlr_virtual_pointer_v1_frame(p); }
static void wait_ms(int ms) { nanosleep(&(struct timespec){ms / 1000, ms % 1000 * 1000000L}, NULL); }
int main(void) {
  struct wl_display *d = wl_display_connect(NULL); if (!d) return 1;
  struct wl_registry *r = wl_display_get_registry(d); wl_registry_add_listener(r, &L, NULL); wl_display_roundtrip(d);
  if (!mgr) { fprintf(stderr, "no mgr\n"); return 2; }
  struct zwlr_virtual_pointer_v1 *p = zwlr_virtual_pointer_manager_v1_create_virtual_pointer(mgr, NULL);
  (void)p; wl_display_roundtrip(d);
  char line[64]; int x, y;
  setvbuf(stdin, NULL, _IOLBF, 0);
  while (fgets(line, sizeof line, stdin)) {
    if (sscanf(line, "m %d %d", &x, &y) == 2) { zwlr_virtual_pointer_v1_motion_absolute(p, now(), x, y, 2560, 1440); zwlr_virtual_pointer_v1_frame(p); }
    else if (sscanf(line, "w %d", &x) == 1) { zwlr_virtual_pointer_v1_axis_source(p, 0); zwlr_virtual_pointer_v1_axis_discrete(p, now(), 0, wl_fixed_from_int(15 * x), x); zwlr_virtual_pointer_v1_frame(p); }
    else if (sscanf(line, "s %d", &x) == 1) wait_ms(x);
    else if (line[0] == 'd') btn(p, 272, 1);
    else if (line[0] == 'u') btn(p, 272, 0);
    else if (line[0] == 'D') btn(p, 273, 1);
    else if (line[0] == 'U') btn(p, 273, 0);
    else if (line[0] == 'c' || line[0] == 'r') {
      uint32_t b = line[0] == 'c' ? 272 : 273;
      btn(p, b, 1); wl_display_roundtrip(d); wait_ms(80); btn(p, b, 0);
    }
    else if (line[0] == 'b' || line[0] == 'f') {
      /* "b [ms [dx]]": hold the side button for ms, moving dx px halfway through */
      int b = line[0] == 'b' ? 275 : 276, ms = 80, dx = 0; sscanf(line + 1, "%d %d", &ms, &dx);
      btn(p, b, 1); wl_display_roundtrip(d); wait_ms(ms / 2);
      if (dx) { zwlr_virtual_pointer_v1_motion(p, now(), wl_fixed_from_int(dx), 0); zwlr_virtual_pointer_v1_frame(p); wl_display_roundtrip(d); }
      wait_ms(ms - ms / 2); btn(p, b, 0);
    }
    wl_display_roundtrip(d);
  }
  return 0;
}
