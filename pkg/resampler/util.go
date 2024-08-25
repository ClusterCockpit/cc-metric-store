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
	flag := 0
	for _, point := range points {
		avgX += util.Float(xStart)
		avgY += point
		xStart++
		if math.IsNaN(float64(point)) {
			flag = 1
		}
	}

	l := util.Float(len(points))

	avgX /= l
	avgY /= l

	if flag == 1 {
		return avgX, util.NaN
	} else {
		return avgX, avgY
	}
}
