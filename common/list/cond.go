package list

func (l List[T]) IsEmpty() bool {
	return l.len == 0
}

func (l *List[T]) PopBack() T {
	if l.len == 0 {
		var defaultValue T
		return defaultValue
	}
	entry := l.root.prev
	l.remove(entry)
	return entry.Value
}

func (l *List[T]) PopFront() T {
	if l.len == 0 {
		var defaultValue T
		return defaultValue
	}
	entry := l.root.next
	l.remove(entry)
	return entry.Value
}

func (l *List[T]) Array() []T {
	if l.len == 0 {
		return nil
	}
	array := make([]T, 0, l.len)
	for element := l.Front(); element != nil; element = element.Next() {
		array = append(array, element.Value)
	}
	return array
}
