package main

// #cgo pkg-config: vpx
//
// #include <string.h>
// #include <vpx/vp8cx.h>
// #include <vpx/vpx_encoder.h>
//
// void rgba2yuv(uint8_t *yuv, uint8_t *rgba, size_t width, size_t height) {
//     size_t i = 0;
//
//     size_t upos = width * height;
//     size_t vpos = upos + upos / 4;
//
//     for (size_t line = 0; line < height; ++line)
//         if (line % 2)
//             for (size_t column = 0; column < width; column += 1) {
//                 uint8_t r = rgba[4 * i];
//                 uint8_t g = rgba[4 * i + 1];
//                 uint8_t b = rgba[4 * i + 2];
//
//                 yuv[i++] = ((66 * r + 129 * g + 25 * b) >> 8) + 16;
//             }
//         else
//             for (size_t column = 0; column < width; column += 2) {
//                 uint8_t r = rgba[4 * i];
//                 uint8_t g = rgba[4 * i + 1];
//                 uint8_t b = rgba[4 * i + 2];
//
//                 yuv[i++] = ((66 * r + 129 * g + 25 * b) >> 8) + 16;
//
//                 yuv[upos++] = ((-38 * r + -74 * g + 112 * b) >> 8) + 128;
//                 yuv[vpos++] = ((112 * r + -94 * g + -18 * b) >> 8) + 128;
//
//                 r = rgba[4 * i];
//                 g = rgba[4 * i + 1];
//                 b = rgba[4 * i + 2];
//
//                 yuv[i++] = ((66 * r + 129 * g + 25 * b) >> 8) + 16;
//             }
// }
//
// int vpx_width(const vpx_image_t *img, int plane) {
//     if (plane <= 0 || img->x_chroma_shift <= 0)
//         return img->d_w;
//
//     return (img->d_w + 1) >> img->x_chroma_shift;
// }
//
// int vpx_height(const vpx_image_t *img, int plane) {
//     if (plane <= 0 || img->y_chroma_shift <= 0)
//         return img->d_h;
//
//     return (img->d_h + 1) >> img->y_chroma_shift;
// }
//
// void yuv2vpx(vpx_image_t *img, uint8_t *yuv) {
//     for (int plane = 0; plane < 3; ++plane) {
//         const int h = vpx_height(img, plane);
//         const int w = vpx_width(img, plane) * ((img->fmt & VPX_IMG_FMT_HIGHBITDEPTH) ? 2 : 1);
//         const int stride = img->stride[plane];
//         unsigned char *buf = img->planes[plane];
//
//         for (int i = 0; i < h; ++i) {
//             memcpy(buf, yuv, w);
//             buf += stride;
//             yuv += w;
//         }
//     }
// }
//
// size_t encode(vpx_codec_ctx_t *ctx, vpx_image_t *img, vpx_codec_pts_t pts, uint64_t flags, void *rgba, void *yuv, size_t w, size_t h, void **fb) {
//     rgba2yuv(yuv, rgba, w, h);
//     yuv2vpx(img, yuv);
//     if (vpx_codec_encode(ctx, img, pts, 1, flags, VPX_DL_REALTIME) != 0)
//         return 0;
//
//     const vpx_codec_cx_pkt_t *pkt = NULL;
//     vpx_codec_iter_t iter = NULL;
//     while ((pkt = vpx_codec_get_cx_data(ctx, &iter)))
//         if (pkt->kind == VPX_CODEC_CX_FRAME_PKT) {
//             *fb = pkt->data.frame.buf;
//             return pkt->data.frame.sz;
//         }
//
//     return 0;
// }
//
// vpx_codec_err_t codec_enc_config_default(vpx_codec_enc_cfg_t *cfg) {
//     return vpx_codec_enc_config_default(vpx_codec_vp8_cx(), cfg, 0);
// }
//
// vpx_codec_err_t codec_enc_init(vpx_codec_ctx_t *codec, vpx_codec_enc_cfg_t *cfg) {
//     return vpx_codec_enc_init(codec, vpx_codec_vp8_cx(), cfg, 0);
// }
//
import "C"

import (
	"bytes"
	"fmt"
	"image"
	"unsafe"
)

const (
	keyFrameInterval = 10
)

type VP8Encoder struct {
	buffer     *bytes.Buffer
	realSize   image.Point
	codecCtx   C.vpx_codec_ctx_t
	vpxImage   C.vpx_image_t
	yuvBuffer  []byte
	frameCount uint
}

func NewVP8Encoder(size image.Point, frameRate int) (*VP8Encoder, error) {
	var codecEncCfg C.vpx_codec_enc_cfg_t
	if C.codec_enc_config_default(&codecEncCfg) != 0 {
		return nil, fmt.Errorf("can't init default enc. config")
	}

	codecEncCfg.g_w = C.uint(size.X)
	codecEncCfg.g_h = C.uint(size.Y)
	codecEncCfg.g_timebase.num = 1
	codecEncCfg.g_timebase.den = C.int(frameRate)
	codecEncCfg.g_error_resilient = 1
	codecEncCfg.rc_target_bitrate = 90000

	var vpxCodecCtx C.vpx_codec_ctx_t
	if C.codec_enc_init(&vpxCodecCtx, &codecEncCfg) != 0 {
		return nil, fmt.Errorf("failed to initialize enc ctx")
	}

	var vpxImage C.vpx_image_t
	if C.vpx_img_alloc(&vpxImage, C.VPX_IMG_FMT_I420, C.uint(size.X), C.uint(size.Y), 0) == nil {
		return nil, fmt.Errorf("can't alloc. vpx image")
	}

	encoder := &VP8Encoder{
		buffer:     bytes.NewBuffer(make([]byte, 0)),
		realSize:   size,
		codecCtx:   vpxCodecCtx,
		vpxImage:   vpxImage,
		yuvBuffer:  make([]byte, size.X*size.Y*2),
		frameCount: 0,
	}
	return encoder, nil
}

func (e *VP8Encoder) Encode(frame *image.RGBA) ([]byte, error) {
	var flags C.uint64_t
	if e.frameCount%keyFrameInterval == 0 {
		flags |= C.VPX_EFLAG_FORCE_KF
	}

	encodedData := unsafe.Pointer(nil)
	frameSize := C.encode(
		&e.codecCtx,
		&e.vpxImage,
		C.vpx_codec_pts_t(e.frameCount),
		flags,
		unsafe.Pointer(&frame.Pix[0]),
		unsafe.Pointer(&e.yuvBuffer[0]),
		C.size_t(e.realSize.X),
		C.size_t(e.realSize.Y),
		&encodedData,
	)

	e.frameCount++

	if int(frameSize) <= 0 {
		return nil, nil
	}

	return C.GoBytes(encodedData, C.int(frameSize)), nil
}

func (e *VP8Encoder) VideoSize() (image.Point, error) {
	return e.realSize, nil
}

func (e *VP8Encoder) Close() error {
	C.vpx_img_free(&e.vpxImage)
	C.vpx_codec_destroy(&e.codecCtx)
	return nil
}
