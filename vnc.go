package main

// #cgo pkg-config: libvncclient
//
// #include <rfb/rfbclient.h>
//
// static unsigned char *__fb_snapshot = NULL;
//
// static unsigned char *get_fb_snapshot() {
//     return __fb_snapshot;
// }
//
// static void set_fb_snapshot(unsigned char *s) {
//     __fb_snapshot = s;
// }
//
// static int get_fb_width(rfbClient *c) {
//     return c->width;
// }
//
// static int get_fb_height(rfbClient *c) {
//     return c->height;
// }
//
// static int get_fb_depth(rfbClient *c) {
//     return c->format.bitsPerPixel;
// }
//
// static int calc_fb_size(rfbClient *c) {
//     return get_fb_width(c) * get_fb_height(c) * get_fb_depth(c) / 8;
// }
//
// static rfbBool malloc_fb(rfbClient *c) {
//     int fb_size = calc_fb_size(c);
//
//     unsigned char *fb = malloc(fb_size * sizeof(unsigned char));
//     if (!fb)
//         goto fail;
//     rfbClientSetClientData(c, NULL, fb);
//
//     unsigned char *fb_snapshot = malloc(fb_size * sizeof(unsigned char));
//     if (!fb_snapshot)
//         goto fail;
//     set_fb_snapshot(fb_snapshot);
//
//     c->frameBuffer = fb;
//     return TRUE;
//
// fail:
//     if (fb) {
//         free(fb);
//         fb = NULL;
//     }
//
//     if (fb_snapshot) {
//         free(fb_snapshot);
//         fb_snapshot = NULL;
//     }
//
//     return FALSE;
// }
//
// static void got_fb_update(rfbClient *c, int x, int y, int w, int h) {
//     int fb_size = calc_fb_size(c);
//
//     unsigned char *fb = rfbClientGetClientData(c, NULL);
//     memcpy(get_fb_snapshot(), fb, fb_size * sizeof(unsigned char));
// }
//
// static rfbClient *rfb_init_client(char addr[]) {
//     static char zero[] = "";
//
//     rfbClient *c = NULL;
//
//     int argc = 2;
//
//     char **argv = malloc(argc * sizeof(char *));
//     argv[0] = zero;
//     argv[1] = addr;
//
//     c = rfbGetClient(8, 3, 4);
//     c->MallocFrameBuffer = malloc_fb;
//     c->GotFrameBufferUpdate = got_fb_update;
//
//     if (!rfbInitClient(c, &argc, argv))
//         goto fail;
//
//     return c;
//
// fail:
//     rfbClientCleanup(c);
//     return NULL;
// }
//
// static void handle_rfb_server_message(rfbClient *c) {
//     int i;
//     for (;;) {
//         i = WaitForMessage(c, 500);
//         if (i < 0)
//             break;
//         if (i && !HandleRFBServerMessage(c))
//             break;
//     }
// }
//
// static rfbBool send_fb_update_request(rfbClient *c) {
//     return SendFramebufferUpdateRequest(c, 0, 0, get_fb_width(c), get_fb_height(c), FALSE);
// }
//
// static void rfb_client_cleanup(rfbClient *c) {
//     rfbClientCleanup(c);
// }
//
import "C"

import (
	"errors"
	"image"
	"sync"
	"unsafe"
)

type VNCClient struct {
	destroyed bool
	destroy   sync.Once
	loop      sync.Once
	addr      *C.char
	rfbClient *C.rfbClient
}

func NewVNCClient(addr string) (*VNCClient, error) {
	if addr == "" {
		addr = "127.0.0.1:5901"
	}

	var vncClient VNCClient

	vncClient.addr = C.CString(addr)
	if vncClient.addr == nil {
		return nil, errors.New("CString")
	}

	var ok bool
	defer func() {
		if !ok {
			C.free(unsafe.Pointer(vncClient.addr))
		}
	}()

	rfbClient := C.rfb_init_client(vncClient.addr)
	if rfbClient == nil {
		return nil, errors.New("rfb_init_client")
	}
	vncClient.rfbClient = rfbClient

	ok = true
	return &vncClient, nil
}

func (c *VNCClient) Loop() {
	c.loop.Do(func() {
		C.handle_rfb_server_message(c.rfbClient)
	})
}

func (c *VNCClient) Destroy() {
	c.destroyed = true
	c.destroy.Do(func() {
		C.rfb_client_cleanup(c.rfbClient)
		C.free(unsafe.Pointer(c.addr))
	})
}

func (c *VNCClient) RequestFrame() (*image.RGBA, error) {
	if c.destroyed {
		return nil, errors.New("destroyed")
	}

	if C.send_fb_update_request(c.rfbClient) == C.FALSE {
		return nil, errors.New("send_fb_update_request")
	}

	fbSnapshot := unsafe.Pointer(C.get_fb_snapshot())
	if fbSnapshot == nil {
		return nil, errors.New("get_fb_snapshot")
	}

	fbWidth := int(C.get_fb_width(c.rfbClient))
	if fbWidth < 0 {
		return nil, errors.New("get_fb_width")
	}

	fbHeight := int(C.get_fb_height(c.rfbClient))
	if fbHeight < 0 {
		return nil, errors.New("get_fb_height")
	}

	fbSize := C.calc_fb_size(c.rfbClient)
	if int(fbSize) < 0 {
		return nil, errors.New("calc_fb_size")
	}

	frame := image.NewRGBA(image.Rect(0, 0, fbWidth, fbHeight))
	copy(frame.Pix, C.GoBytes(fbSnapshot, fbSize))

	return frame, nil
}

type VNCFrameProvider struct {
	client *VNCClient
}

var _ FrameProvider = (*VNCFrameProvider)(nil)

func NewVNCFrameProvider(addr string) (*VNCFrameProvider, error) {
	client, err := NewVNCClient(addr)
	if err != nil {
		return nil, err
	}
	go client.Loop()

	provider := VNCFrameProvider{
		client: client,
	}
	return &provider, nil
}

func (p *VNCFrameProvider) Frame() (*image.RGBA, error) {
	return p.client.RequestFrame()
}

func (p *VNCFrameProvider) Close() error {
	p.client.Destroy()
	return nil
}

type VNCFrameProviderFactory struct {
	Addr string
}

var _ FrameProviderFactory = (*VNCFrameProviderFactory)(nil)

func (f *VNCFrameProviderFactory) NewFrameProvider() (FrameProvider, error) {
	return NewVNCFrameProvider(f.Addr)
}
