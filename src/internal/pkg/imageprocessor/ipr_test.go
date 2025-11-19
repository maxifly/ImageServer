package imageprocessor

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isBlack проверяет, что цвет — чёрный (с небольшим допуском)
func isBlack(c color.Color) bool {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return rgba.R <= 5 && rgba.G <= 5 && rgba.B <= 5 && rgba.A >= 250
}

// isRed проверяет, что цвет — красный
func isRed(c color.Color) bool {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return rgba.R >= 250 && rgba.G <= 5 && rgba.B <= 5 && rgba.A >= 250
}

// encodeToJPEG кодирует image.Image в []byte (JPEG)
func encodeToJPEG(img image.Image) []byte {
	buf := new(bytes.Buffer)
	err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 90})
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// createRedImage создаёт красное изображение заданного размера
func createRedImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	red := color.RGBA{255, 0, 0, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{red}, image.Point{}, draw.Src)
	return img
}

// TestNewIpr создаёт экземпляр Ipr для тестов
func newTestIpr() *Ipr {
	return &Ipr{
		imageParameters: ImageParameters{
			ImageHeight:  100,
			ImageWeight:  100,
			FitThreshold: 0.02,
		},
		logger: slog.Default(),
	}
}

// ========================================
// ТЕСТ: вертикальные чёрные полосы (широкое → квадрат)
// ========================================
func TestIpr_ProcessImage_VerticalPadding(t *testing.T) {
	ipr := newTestIpr()

	// Широкое изображение: 400x200 → целевой: 200x200
	src := createRedImage(400, 200)
	imgData := encodeToJPEG(src)

	processed, _, err := ipr.ProcessImageFromSLice(imgData, 200, 200, false)
	require.NoError(t, err)
	assert.NotEmpty(t, processed)

	// Декодируем результат
	result, _, err := image.Decode(bytes.NewReader(processed))
	require.NoError(t, err)

	b := result.Bounds()
	assert.Equal(t, 200, b.Dx())
	assert.Equal(t, 200, b.Dy())

	// После Fit: 400x200 → 200x100 → Pad добавит 50px сверху и снизу
	assert.True(t, isRed(result.At(100, 100)), "center must be red")
	assert.True(t, isBlack(result.At(100, 10)), "top must be black")
	assert.True(t, isBlack(result.At(100, 190)), "bottom must be black")
}

// ========================================
// ТЕСТ: горизонтальные чёрные полосы (высокое → квадрат)
// ========================================
func TestIpr_ProcessImage_HorizontalPadding(t *testing.T) {
	ipr := newTestIpr()

	// Высокое изображение: 200x400 → целевой: 200x200
	src := createRedImage(200, 400)
	imgData := encodeToJPEG(src)

	processed, _, err := ipr.ProcessImageFromSLice(imgData, 200, 200, false)
	require.NoError(t, err)
	assert.NotEmpty(t, processed)

	result, _, err := image.Decode(bytes.NewReader(processed))
	require.NoError(t, err)

	b := result.Bounds()
	assert.Equal(t, 200, b.Dx())
	assert.Equal(t, 200, b.Dy())

	// После Fit: 200x400 → 100x200 → Pad добавит 50px слева и справа
	assert.True(t, isRed(result.At(100, 100)), "center must be red")
	assert.True(t, isBlack(result.At(10, 100)), "left must be black")
	assert.True(t, isBlack(result.At(190, 100)), "right must be black")
}

// ========================================
// ТЕСТ: точное совпадение размеров
// ========================================
func TestIpr_ProcessImage_ExactSize(t *testing.T) {
	ipr := newTestIpr()

	src := createRedImage(300, 200)
	imgData := encodeToJPEG(src)

	processed, original, err := ipr.ProcessImageFromSLice(imgData, 300, 200, true)
	require.NoError(t, err)
	assert.NotEmpty(t, processed)
	assert.NotEmpty(t, original)

	// Оба должны быть 300x200
	for _, data := range [][]byte{processed, original} {
		img, _, err := image.Decode(bytes.NewReader(data))
		require.NoError(t, err)
		b := img.Bounds()
		assert.Equal(t, 300, b.Dx())
		assert.Equal(t, 200, b.Dy())
	}
}

// ========================================
// ТЕСТ: wantOriginal = false → второй результат nil
// ========================================
func TestIpr_ProcessImage_NoOriginal(t *testing.T) {
	ipr := newTestIpr()

	src := createRedImage(100, 100)
	imgData := encodeToJPEG(src)

	processed, original, err := ipr.ProcessImageFromSLice(imgData, 150, 150, false)
	require.NoError(t, err)
	assert.NotEmpty(t, processed)
	assert.Nil(t, original) // ← важно!
}

// ========================================
// ТЕСТ: wantOriginal = true → второй результат не nil
// ========================================
func TestIpr_ProcessImage_WithOriginal(t *testing.T) {
	ipr := newTestIpr()

	src := createRedImage(100, 100)
	imgData := encodeToJPEG(src)

	processed, original, err := ipr.ProcessImageFromSLice(imgData, 150, 150, true)
	require.NoError(t, err)
	assert.NotEmpty(t, processed)
	assert.NotNil(t, original)
	assert.NotEmpty(t, original)
}

// ========================================
// ТЕСТ: невалидные входные данные
// ========================================
func TestIpr_ProcessImage_InvalidInput(t *testing.T) {
	ipr := newTestIpr()

	_, _, err := ipr.ProcessImageFromSLice([]byte("not an image"), 100, 100, false)
	assert.Error(t, err)
}

// ========================================
// ТЕСТ: малое отклонение → растягиваем без полос
// ========================================
func TestIpr_ProcessImage_SmallRatioDiff_ResizeWithoutPadding(t *testing.T) {
	ipr := &Ipr{
		imageParameters: ImageParameters{
			ImageHeight:  100,
			ImageWeight:  100,
			FitThreshold: 0.1,
		},
		logger: slog.Default(),
	}

	// Исходное: 1000x800 (ratio=1.25), целевое: 500x410 (ratio≈1.22) → diff ≈ 2.4% < 10%
	src := createRedImage(1000, 800)
	imgData := encodeToJPEG(src)

	processed, _, err := ipr.ProcessImageFromSLice(imgData, 500, 410, false)
	require.NoError(t, err)

	result, _, err := image.Decode(bytes.NewReader(processed))
	require.NoError(t, err)

	// Должно быть ТОЧНО 500x410 — без полос!
	b := result.Bounds()
	assert.Equal(t, 500, b.Dx())
	assert.Equal(t, 410, b.Dy())

	// Углы не чёрные → значит, не было pad
	assert.False(t, isBlack(result.At(10, 10)), "should not have black padding")
}
