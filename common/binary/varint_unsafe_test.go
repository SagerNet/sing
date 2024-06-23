package binary

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlicesValue(t *testing.T) {
	int64Arr := make([]int64, 64)
	for i := range int64Arr {
		int64Arr[i] = rand.Int64()
	}
	require.Equal(t, int64Arr, slicesValue[int64](reflect.ValueOf(int64Arr)))
	require.Equal(t, int64Arr, baseDataSlices(reflect.ValueOf(int64Arr)))
}

func TestSetSliceValue(t *testing.T) {
	int64Arr := make([]int64, 64)
	value := reflect.Indirect(reflect.ValueOf(&int64Arr))
	newInt64Arr := make([]int64, 64)
	for i := range newInt64Arr {
		newInt64Arr[i] = rand.Int64()
	}
	setSliceValue[int64](value, newInt64Arr)
	require.Equal(t, newInt64Arr, slicesValue[int64](value))
	newInt64Arr2 := makeBaseDataSlices(value, 64)
	copy(newInt64Arr2.([]int64), newInt64Arr)
	require.Equal(t, newInt64Arr, newInt64Arr2)
	value.SetZero()
	setBaseDataSlices(value, newInt64Arr2)
	require.Equal(t, newInt64Arr, slicesValue[int64](value))
}
