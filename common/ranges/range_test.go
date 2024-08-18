package ranges

import (
	"reflect"
	"testing"
)

func TestRevertRanges(t *testing.T) {
	t.Parallel()
	for _, testRange := range []struct {
		start, end int
		ranges     []Range[int]
		expected   []Range[int]
	}{
		{
			start: 0,
			end:   10,
			ranges: []Range[int]{
				{0, 1},
			},
			expected: []Range[int]{
				{2, 10},
			},
		},
		{
			start: 0,
			end:   10,
			ranges: []Range[int]{
				{9, 10},
			},
			expected: []Range[int]{
				{0, 8},
			},
		},
		{
			start: 0,
			end:   10,
			ranges: []Range[int]{
				{0, 1},
				{9, 10},
			},
			expected: []Range[int]{
				{2, 8},
			},
		},
		{
			start: 0,
			end:   10,
			ranges: []Range[int]{
				{2, 4},
				{6, 8},
			},
			expected: []Range[int]{
				{0, 1},
				{5, 5},
				{9, 10},
			},
		},
		{
			start: 0,
			end:   10,
			ranges: []Range[int]{
				{2, 4},
				{8, 9},
			},
			expected: []Range[int]{
				{0, 1},
				{5, 7},
				{10, 10},
			},
		},
	} {
		result := Revert(testRange.start, testRange.end, testRange.ranges)
		if !reflect.DeepEqual(result, testRange.expected) {
			t.Fatal("expected", testRange.expected, "\ngot", result)
		}
	}
}

func TestMergeRanges(t *testing.T) {
	t.Parallel()
	for _, testRange := range []struct {
		ranges   []Range[int]
		expected []Range[int]
	}{
		{
			ranges: []Range[int]{
				{0, 1},
				{1, 2},
			},
			expected: []Range[int]{
				{0, 2},
			},
		},
		{
			ranges: []Range[int]{
				{0, 3},
				{5, 7},
				{8, 9},
				{10, 10},
			},
			expected: []Range[int]{
				{0, 3},
				{5, 10},
			},
		},
		{
			ranges: []Range[int]{
				{1, 3},
				{2, 6},
				{8, 10},
				{15, 18},
			},
			expected: []Range[int]{
				{1, 6},
				{8, 10},
				{15, 18},
			},
		},
		{
			ranges: []Range[int]{
				{1, 3},
				{2, 7},
				{2, 6},
			},
			expected: []Range[int]{
				{1, 7},
			},
		},
		{
			ranges: []Range[int]{
				{1, 3},
				{2, 6},
				{2, 7},
			},
			expected: []Range[int]{
				{1, 7},
			},
		},
	} {
		result := Merge(testRange.ranges)
		if !reflect.DeepEqual(result, testRange.expected) {
			t.Fatal("input", testRange.ranges, "\nexpected", testRange.expected, "\ngot", result)
		}
	}
}

func TestExcludeRanges(t *testing.T) {
	t.Parallel()
	for _, testRange := range []struct {
		ranges   []Range[int]
		exclude  []Range[int]
		expected []Range[int]
	}{
		{
			ranges: []Range[int]{
				{0, 100},
			},
			exclude: []Range[int]{
				{0, 10},
				{20, 30},
				{55, 55},
			},
			expected: []Range[int]{
				{11, 19},
				{31, 54},
				{56, 100},
			},
		},
		{
			ranges: []Range[int]{
				{0, 100},
				{200, 300},
			},
			exclude: []Range[int]{
				{0, 10},
				{20, 30},
				{55, 55},
				{250, 250},
				{299, 299},
			},
			expected: []Range[int]{
				{11, 19},
				{31, 54},
				{56, 100},
				{200, 249},
				{251, 298},
				{300, 300},
			},
		},
	} {
		result := Exclude(testRange.ranges, testRange.exclude)
		if !reflect.DeepEqual(result, testRange.expected) {
			t.Fatal("input", testRange.ranges, testRange.exclude, "\nexpected", testRange.expected, "\ngot", result)
		}
	}
}
