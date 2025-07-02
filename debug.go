package glay

import "log"

// Add this to your project (potentially in a debug.go file)
func (context *Context) ClayWithDebug(decl ElementDeclaration, declChildren ...func() error) error {
	log.Printf("Clay called with ID: %s", decl.ID.StringID)
	log.Printf("Current arrays: LayoutElements len=%d cap=%d, LayoutElementChildren len=%d cap=%d",
		len(context.LayoutElements), cap(context.LayoutElements),
		len(context.LayoutElementChildren), cap(context.LayoutElementChildren))

	err := context.OpenElement()
	if err != nil {
		log.Printf("openElement failed: %v", err)
		return err
	}

	err = context.ConfigureOpenElement(decl)
	if err != nil {
		log.Printf("configureOpenElement failed: %v", err)
		return err
	}

	log.Printf("After configureOpenElement: LayoutElements len=%d cap=%d",
		len(context.LayoutElements), cap(context.LayoutElements))

	for i, declCh := range declChildren {
		log.Printf("Processing child function %d/%d for parent %s",
			i+1, len(declChildren), decl.ID.StringID)
		err = declCh()
		if err != nil {
			log.Printf("Child function %d failed: %v", i+1, err)
			return err
		}
	}

	log.Printf("Before closeElement: LayoutElements len=%d cap=%d, LayoutElementChildren len=%d cap=%d",
		len(context.LayoutElements), cap(context.LayoutElements),
		len(context.LayoutElementChildren), cap(context.LayoutElementChildren))

	return context.CloseElement()
}
