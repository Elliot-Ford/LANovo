// Android 10 shim for libcountertop's Android 9 surface helper, preloaded on
// the Lenovo SmartClock 2 (helios). libcountertop only builds the helper for
// API 25/27/28; the clock's libgui is API 29, which dropped or re-signed five
// of the symbols the helper links against, and grew Transaction by a third
// unordered_map that the helper's hand-built Transaction leaves invalid.
//
// Each export below keeps the Android 9 mangled name and forwards to the
// Android 10 entry point. Build with the NDK armv7 clang:
//   clang -shared -fPIC -O2 -o liblanovo-shim.so shim.c
// and run the helper with LD_PRELOAD pointing at it.
//
// Layouts are from AOSP android-10.0.0_r1 (libs/gui, external/libcxx) on
// 32-bit ARM, and the DisplayState stride (0x3c) was read from the clock's own
// libgui (SortedVector<DisplayState>::do_destroy).

#include <stdint.h>
#include <string.h>

#define EXPORT __attribute__((visibility("default")))

// ---- Android 10 entry points ------------------------------------------------
//
// Declared weak, so the preload also loads into processes without libgui
// (LD_PRELOAD reaches nohup and the shell too); in the helper they bind to
// libgui/libui.


extern __attribute__((weak)) void a10_internal_display(void **sret)
	__asm__("_ZN7android21SurfaceComposerClient23getInternalDisplayTokenEv");

extern __attribute__((weak)) void a10_create_surface(void **sret, void *client, const void *name, uint32_t w, uint32_t h,
	int32_t format, uint32_t flags, void *parent, void *metadata)
	__asm__("_ZN7android21SurfaceComposerClient13createSurfaceERKNS_7String8EjjijPNS_14SurfaceControlENS_13LayerMetadataE");

extern __attribute__((weak)) void *a10_metadata_ctor(void *self) __asm__("_ZN7android13LayerMetadataC1Ev");


extern __attribute__((weak)) int a10_gb_lock(void *self, uint32_t usage, void **bits, int32_t *bpp, int32_t *bps)
	__asm__("_ZN7android13GraphicBuffer4lockEjPPvPiS3_");

extern __attribute__((weak)) void *a10_tx_copy(void *self, const void *from)
	__asm__("_ZN7android21SurfaceComposerClient11TransactionC2ERKS1_");

// ---- Android 9 names the helper links against --------------------------------

// static sp<IBinder> getBuiltInDisplay(int32_t id); id 0 is the built-in panel.
EXPORT void a9_builtin_display(void **sret, int32_t id)
	__asm__("_ZN7android21SurfaceComposerClient17getBuiltInDisplayEi");
void a9_builtin_display(void **sret, int32_t id) {
	(void)id;
	a10_internal_display(sret);
}

// createSurface(name, w, h, format, flags, parent, int32 windowType, int32 ownerUid)
// -> createSurface(name, w, h, format, flags, parent, LayerMetadata).
// LayerMetadata is passed by value: the caller owns a copy and passes its
// address (AAPCS, non-trivial type). It is a Parcelable vptr plus an empty
// unordered_map, so a constructed one leaks nothing when not destroyed; the
// callee's copy is what SurfaceFlinger reads.
EXPORT void a9_create_surface(void **sret, void *client, const void *name, uint32_t w, uint32_t h,
	int32_t format, uint32_t flags, void *parent, int32_t wtype, int32_t uid)
	__asm__("_ZN7android21SurfaceComposerClient13createSurfaceERKNS_7String8EjjijPNS_14SurfaceControlEii");
void a9_create_surface(void **sret, void *client, const void *name, uint32_t w, uint32_t h,
	int32_t format, uint32_t flags, void *parent, int32_t wtype, int32_t uid) {
	(void)wtype;
	(void)uid;
	uint64_t metadata[8];
	memset(metadata, 0, sizeof metadata);
	a10_metadata_ctor(metadata);
	a10_create_surface(sret, client, name, w, h, format, flags, parent, metadata);
}

// Android 10 has no SurfaceControl::clear(); the destructor (run when the
// helper drops its last strong reference right after) reparents the layer
// off-screen and releases the handle, which is what clear() did.
EXPORT void a9_sc_clear(void *sc) __asm__("_ZN7android14SurfaceControl5clearEv");
void a9_sc_clear(void *sc) { (void)sc; }

// capture(display, Rect crop, reqW, reqH, minZ, maxZ, identity, rotation, out)
// -> capture(display, Dataspace, PixelFormat, Rect crop, reqW, reqH, identity, rotation, out).
// Rect is a trivially copyable 16-byte struct, passed by value in core
// registers and the stack exactly like the four int32s the helper passes.
typedef struct { int32_t left, top, right, bottom; } rect;
extern __attribute__((weak)) int a10_capture_rect(const void *display, int32_t dataspace, int32_t format, rect crop,
	uint32_t w, uint32_t h, int identity, uint32_t rotation, void **out)
	__asm__("_ZN7android16ScreenshotClient7captureERKNS_2spINS_7IBinderEEENS_8hardware8graphics6common4V1_29DataspaceENS9_11PixelFormatENS_4RectEjjbjPNS1_INS_13GraphicBufferEEE");

#define DATASPACE_V0_SRGB 0x8810000 /* ui::Dataspace::V0_SRGB: BT709 | sRGB transfer | full range */
#define PIXEL_FORMAT_RGBA_8888 1

EXPORT int a9_capture(const void *display, rect crop, uint32_t w, uint32_t h,
	int32_t min_z, int32_t max_z, int identity, uint32_t rotation, void **out)
	__asm__("_ZN7android16ScreenshotClient7captureERKNS_2spINS_7IBinderEEENS_4RectEjjiibjPNS1_INS_13GraphicBufferEEE");
int a9_capture(const void *display, rect crop, uint32_t w, uint32_t h,
	int32_t min_z, int32_t max_z, int identity, uint32_t rotation, void **out) {
	(void)min_z;
	(void)max_z;
	return a10_capture_rect(display, DATASPACE_V0_SRGB, PIXEL_FORMAT_RGBA_8888, crop, w, h, identity,
		rotation, out);
}

// lock(usage, void **bits) -> lock(usage, bits, int *bytesPerPixel, int *bytesPerStride).
EXPORT int a9_gb_lock(void *self, uint32_t usage, void **bits) __asm__("_ZN7android13GraphicBuffer4lockEjPPv");
int a9_gb_lock(void *self, uint32_t usage, void **bits) {
	int32_t bpp = 0, bps = 0;
	return a10_gb_lock(self, usage, bits, &bpp, &bps);
}

// ---- Transaction copy constructor (interposed, the name exists on A10) ------
//
// The helper builds its one long-lived Transaction by zeroing a blob, setting
// the Fire OS 7 fields it knows (mComposerStates.max_load_factor at 0x14 and
// mDisplayStates' item size at 0x28) and copy-constructing from it. Android 10
// lays out:
//   0x00 vptr
//   0x04 mComposerStates     unordered_map (20 bytes; max_load_factor at +0x10)
//   0x18 mDisplayStates      SortedVector<DisplayState> (item size at +0x10)
//   0x2c mListenerCallbacks  unordered_map (max_load_factor at 0x3c)
// so the first two line up, but DisplayState is 0x3c bytes here (VectorImpl
// aborts on an item size mismatch), and mListenerCallbacks' load factor would
// be 0, which makes the first insertion (every setter registers the layer for
// callbacks) rehash to ceil(1/0). Fix both on the source before copying.
EXPORT void *a9_tx_copy(void *self, const void *from)
	__asm__("_ZN7android21SurfaceComposerClient11TransactionC1ERKS1_");
void *a9_tx_copy(void *self, const void *from) {
	uint32_t *src = (uint32_t *)from;
	float lf;
	memcpy(&lf, (char *)src + 0x3c, sizeof lf);
	if (src[0] == 0 && src[0x28 / 4] == 0x40) {
		const float one = 1.0f;
		src[0x28 / 4] = 0x3c;
		if (lf == 0.0f) memcpy((char *)src + 0x3c, &one, sizeof one);
	}
	return a10_tx_copy(self, from);
}
