package ask

import "context"

// cmdLink links a command to its description and its parent command
type cmdLink struct {
	Parent *cmdLink
	Descr  *cmdDescription
	Cmd    Command
}

type ctxLinkKeyType struct{}

var ctxLinkKey = ctxLinkKeyType{}

func getCmdLink(ctx context.Context) *cmdLink {
	v := ctx.Value(ctxLinkKey)
	if v == nil {
		return nil
	}
	return v.(*cmdLink)
}

func addCmdLink(ctx context.Context, descr *cmdDescription, cmd Command) context.Context {
	parent := getCmdLink(ctx)
	li := &cmdLink{
		Parent: parent,
		Descr:  descr,
		Cmd:    cmd,
	}
	return context.WithValue(ctx, ctxLinkKey, li)
}
