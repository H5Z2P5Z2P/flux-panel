package captcha

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/image/draw"
)

type SliderTemplate struct {
	SliderMask     *image.NRGBA
	BackgroundMask *image.NRGBA
	Width          int
	Height         int
}

type Background struct {
	Image  image.Image
	Width  int
	Height int
}

type SliderCaptcha struct {
	BackgroundImageBase64 string
	TemplateImageBase64   string
	Width                 int
	Height                int
	TemplateWidth         int
	TemplateHeight        int
	TargetX               int
	TargetY               int
}

type Generator struct {
	backgrounds []*Background
	templates   []SliderTemplate
	bgWidth     int
	bgHeight    int
	rand        *rand.Rand
}

func NewGenerator(bgDir, slideDir string, bgWidth int) (*Generator, error) {
	bgList, err := loadBackgrounds(bgDir, bgWidth)
	if err != nil {
		return nil, err
	}

	templates, err := loadTemplates(slideDir)
	if err != nil {
		return nil, err
	}

	if len(bgList) == 0 || len(templates) == 0 {
		return nil, fmt.Errorf("captcha assets not found")
	}

	return &Generator{
		backgrounds: bgList,
		templates:   templates,
		bgWidth:     bgList[0].Width,
		bgHeight:    bgList[0].Height,
		rand:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func loadBackgrounds(dir string, targetWidth int) ([]*Background, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var list []*Background
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		img, err := loadImage(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		resized := resizeWidth(img, targetWidth)
		list = append(list, &Background{Image: resized, Width: resized.Bounds().Dx(), Height: resized.Bounds().Dy()})
	}
	return list, nil
}

func loadTemplates(dir string) ([]SliderTemplate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var templates []SliderTemplate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		mask1, err := loadImage(filepath.Join(dir, entry.Name(), "1.png"))
		if err != nil {
			continue
		}
		mask2, err := loadImage(filepath.Join(dir, entry.Name(), "2.png"))
		if err != nil {
			continue
		}
		templates = append(templates, SliderTemplate{
			SliderMask:     toNRGBA(mask1),
			BackgroundMask: toNRGBA(mask2),
			Width:          mask1.Bounds().Dx(),
			Height:         mask1.Bounds().Dy(),
		})
	}
	return templates, nil
}

func loadImage(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}

func resizeWidth(img image.Image, width int) *image.NRGBA {
	if img.Bounds().Dx() == width {
		return toNRGBA(img)
	}
	aspect := float64(img.Bounds().Dy()) / float64(img.Bounds().Dx())
	height := int(float64(width) * aspect)
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

func toNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba
	}
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	imagedraw.Draw(dst, b, img, b.Min, imagedraw.Src)
	return dst
}

func (g *Generator) randomBackground() *Background {
	return g.backgrounds[g.rand.Intn(len(g.backgrounds))]
}

func (g *Generator) randomTemplate() SliderTemplate {
	return g.templates[g.rand.Intn(len(g.templates))]
}

func (g *Generator) GenerateSlider() (*SliderCaptcha, error) {
	bg := g.randomBackground()
	tpl := g.randomTemplate()

	sliderWidth := tpl.Width
	sliderHeight := tpl.Height

	margin := 10
	if bg.Width <= sliderWidth+margin*2 || bg.Height <= sliderHeight+margin*2 {
		return nil, fmt.Errorf("background too small")
	}

	targetX := g.rand.Intn(bg.Width-sliderWidth-2*margin) + margin
	targetY := g.rand.Intn(bg.Height-sliderHeight-2*margin) + margin

	bgImg := image.NewNRGBA(bg.Image.Bounds())
	draw.Draw(bgImg, bgImg.Bounds(), bg.Image, image.Point{}, draw.Src)

	sliderImg := image.NewNRGBA(image.Rect(0, 0, sliderWidth, sliderHeight))

	for x := 0; x < sliderWidth; x++ {
		for y := 0; y < sliderHeight; y++ {
			maskAlpha := tpl.SliderMask.NRGBAAt(x, y).A
			bgX := targetX + x
			bgY := targetY + y
			if maskAlpha > 10 {
				color := bgImg.NRGBAAt(bgX, bgY)
				sliderImg.SetNRGBA(x, y, color)
				shade := tpl.BackgroundMask.NRGBAAt(x, y)
				bgImg.SetNRGBA(bgX, bgY, colorWithShade(color, shade.A))
			} else {
				sliderImg.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
			}
		}
	}

	bgBase64, err := encodeBase64(bgImg)
	if err != nil {
		return nil, err
	}
	sliderBase64, err := encodeBase64(sliderImg)
	if err != nil {
		return nil, err
	}

	return &SliderCaptcha{
		BackgroundImageBase64: bgBase64,
		TemplateImageBase64:   sliderBase64,
		Width:                 bg.Width,
		Height:                bg.Height,
		TemplateWidth:         sliderWidth,
		TemplateHeight:        sliderHeight,
		TargetX:               targetX,
		TargetY:               targetY,
	}, nil
}

func colorWithShade(c color.NRGBA, alpha uint8) color.NRGBA {
	if alpha == 0 {
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0}
	}
	factor := float64(alpha) / 255.0
	gray := uint8((float64(c.R)+float64(c.G)+float64(c.B))/3 * 0.7)
	return color.NRGBA{
		R: uint8(float64(c.R)*(1-factor) + float64(gray)*factor),
		G: uint8(float64(c.G)*(1-factor) + float64(gray)*factor),
		B: uint8(float64(c.B)*(1-factor) + float64(gray)*factor),
		A: 255,
	}
}

func encodeBase64(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
