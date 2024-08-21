package resampler

import (
	"math"

	"github.com/ClusterCockpit/cc-metric-store/internal/util"
)

func calculateTriangleArea(paX, paY, pbX, pbY, pcX, pcY util.Float) float64 {
	area := ((paX-pcX)*(pbY-paY) - (paX-pbX)*(pcY-paY)) * 0.5
	return math.Abs(float64(area))
}

func calculateAverageDataPoint(points []util.Float, xStart int64) (avgX util.Float, avgY util.Float) {

	for _, point := range points {
		avgX += util.Float(xStart)
		avgY += point
		xStart++
	}
	l := util.Float(len(points))
	avgX /= l
	avgY /= l
	return avgX, avgY
}
