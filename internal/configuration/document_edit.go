// SPDX-License-Identifier: GPL-3.0-or-later

package configuration

// Map returns a defensive copy of the complete canonical envelope.
func (document *Document) Map() map[string]any {
	if document == nil {
		return nil
	}
	value, err := clonePointerValue(document.ConfigurationEnvelope())
	if err != nil {
		panic(err)
	}
	return value.(map[string]any)
}

func (document *Document) ConfigurationEnvelope() map[string]any {
	if document == nil {
		return nil
	}
	var root map[string]any
	if err := decodeCanonicalMap(document.canonical, &root); err != nil {
		panic(err)
	}
	return root
}

func (document *Document) ValueAtPointer(pointer string) (any, error) {
	if document == nil {
		return nil, ErrInvalidDocument
	}
	tokens, err := parseEditPointer(pointer)
	if err != nil {
		return nil, err
	}
	var current any = document.ConfigurationEnvelope()
	for _, token := range tokens {
		current, err = pointerChild(current, token)
		if err != nil {
			return nil, err
		}
	}
	return clonePointerValue(current)
}

func (document *Document) SetPointer(pointer string, value any) (*Document, error) {
	if document == nil {
		return nil, ErrInvalidDocument
	}
	tokens, err := parseEditPointer(pointer)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, ErrInvalidDocument
		}
		return buildEdited(object)
	}
	root := document.ConfigurationEnvelope()
	parent, err := pointerParent(root, tokens[:len(tokens)-1])
	if err != nil {
		return nil, err
	}
	cloned, err := clonePointerValue(value)
	if err != nil {
		return nil, err
	}
	last := tokens[len(tokens)-1]
	switch typed := parent.(type) {
	case map[string]any:
		typed[last] = cloned
	case []any:
		index, indexErr := pointerIndex(last, len(typed))
		if indexErr != nil {
			return nil, indexErr
		}
		typed[index] = cloned
	default:
		return nil, ErrPointerNotFound
	}
	return buildEdited(root)
}

func (document *Document) UnsetPointer(pointer string) (*Document, error) {
	if document == nil {
		return nil, ErrInvalidDocument
	}
	tokens, err := parseEditPointer(pointer)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, ErrInvalidDocument
	}
	root := document.ConfigurationEnvelope()
	parent, err := pointerParent(root, tokens[:len(tokens)-1])
	if err != nil {
		return nil, err
	}
	last := tokens[len(tokens)-1]
	switch typed := parent.(type) {
	case map[string]any:
		if _, found := typed[last]; !found {
			return nil, ErrPointerNotFound
		}
		delete(typed, last)
	case []any:
		index, indexErr := pointerIndex(last, len(typed))
		if indexErr != nil {
			return nil, indexErr
		}
		if err := replaceArrayAtParent(root, tokens[:len(tokens)-1], append(typed[:index], typed[index+1:]...)); err != nil {
			return nil, err
		}
	default:
		return nil, ErrPointerNotFound
	}
	return buildEdited(root)
}

func buildEdited(root map[string]any) (*Document, error) {
	encoded, err := encodeCanonicalMap(root)
	if err != nil {
		return nil, err
	}
	return Parse(encoded)
}
