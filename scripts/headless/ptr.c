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
    else if (line[0] == 'd') { zwlr_virtual_pointer_v1_button(p, now(), 272, 1); zwlr_virtual_pointer_v1_frame(p); }
    else if (line[0] == 'u') { zwlr_virtual_pointer_v1_button(p, now(), 272, 0); zwlr_virtual_pointer_v1_frame(p); }
    else if (line[0] == 'c') { zwlr_virtual_pointer_v1_button(p, now(), 272, 1); zwlr_virtual_pointer_v1_frame(p); wl_display_roundtrip(d); nanosleep(&(struct timespec){0, 80000000}, NULL); zwlr_virtual_pointer_v1_button(p, now(), 272, 0); zwlr_virtual_pointer_v1_frame(p); }
    wl_display_roundtrip(d);
  }
  return 0;
}
