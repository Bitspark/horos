package wire

import (
	ground "github.com/Bitspark/horos/core/go/declarations"
	model "github.com/Bitspark/horos/core/go/internal/declmodel"
	core "github.com/bitspark/ontos/core/go"
)

// Embed specializes a ground declaration and embeds its closed descriptor in
// Data. It preserves the ground domain and creates no optional declaration.
func Embed(source *ground.Program, id ground.DeclarationID, arguments []core.Value, limits Limits) (core.Value, error) {
	if source == nil {
		return nil, model.Failure(InvalidOptions, "missing ground program")
	}
	root, ok := source.Module(source.Root())
	if !ok {
		return nil, model.Failure(InvalidOptions, "unresolved ground program")
	}
	var dependencies []core.Value
	for _, module := range source.Modules() {
		if module.Pin() != source.Root() {
			dependencies = append(dependencies, module.Value())
		}
	}
	checked, err := ground.ResolveWithLimits(root.Value(), dependencies, limits)
	if err != nil {
		return nil, err
	}
	value, err := checked.Specialize(id, arguments)
	if err != nil {
		return nil, err
	}
	result := tagged("data", value)
	if err := WF(result, limits); err != nil {
		return nil, err
	}
	return result, nil
}
