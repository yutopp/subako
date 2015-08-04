package subako

import (
    "github.com/jinzhu/gorm"
)

type Webhook struct {
	gorm.Model
	Target		string
	Secret		string
	ProcName	string
	Version		string
}

type WebhookContext struct {
	Db		gorm.DB
}

func MakeWebhooksContext(db gorm.DB) (*WebhookContext, error) {
	db.AutoMigrate(&Webhook{})

	return &WebhookContext{
		Db: db,
	}, nil
}

func (ctx *WebhookContext) GetWebhooks() []Webhook {
	webhooks := []Webhook{}
	ctx.Db.Debug().Find(&webhooks)

	return webhooks
}

func (ctx *WebhookContext) Append(hook *Webhook) error {
	// TODO: error handling
	ctx.Db.Debug().Create(hook)

	return nil
}

func (ctx *WebhookContext) Update(id uint, hook *Webhook) error {
	// TODO: error handling
	hook.ID = id
	ctx.Db.Debug().Save(hook)

	return nil
}

func (ctx *WebhookContext) Delete(id uint) error {
	// TODO: error handling
	hook := &Webhook{}
	hook.ID = id

	ctx.Db.Debug().Delete(hook)

	return nil
}

func (ctx *WebhookContext) GetByTarget(target string) (*Webhook, error) {
	// TODO: error handling
	hook := Webhook{}
	query := ctx.Db.Debug().Where(&Webhook{Target: target}).First(&hook)
	if query.Error != nil {
		return nil, query.Error
	}

	return &hook, nil
}

func (ctx *WebhookContext) GetByTargetOrCreate(
	target	string,
	hook	*Webhook,
) (*Webhook, error) {
	got_hook, err := ctx.GetByTarget(target)
	if err != nil {
		if err == gorm.RecordNotFound {
			if err := ctx.Append(hook); err != nil {
				return nil, err
			}
			return hook, nil
		} else {
			return nil, err
		}
	}

	return got_hook, nil
}
