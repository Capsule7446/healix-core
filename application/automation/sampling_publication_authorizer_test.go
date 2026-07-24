package automation

import "context"

type forceCreateAuthorizerFake struct {
	err    error
	called bool
	intent ForceCreateAuthorizationIntent
}

func (fake *forceCreateAuthorizerFake) AuthorizeForceCreate(_ context.Context, intent ForceCreateAuthorizationIntent) error {
	fake.called = true
	fake.intent = intent
	return fake.err
}
