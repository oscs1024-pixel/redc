package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"red-cloud/i18n"
	redc "red-cloud/mod"
	"red-cloud/mod/ai"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AIChatMessage represents a single message in the AI chat conversation
type AIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AIChatStream handles multi-turn AI chat with streaming responses
func (a *App) AIChatStream(conversationId, mode string, messages []AIChatMessage) error {
	// Validate AI config
	profile, err := redc.GetActiveProfile()
	if err != nil || profile.AIConfig == nil {
		return fmt.Errorf("%s", i18n.T("app_ai_not_configured"))
	}

	aiConfig := profile.AIConfig
	if aiConfig.APIKey == "" || aiConfig.BaseURL == "" || aiConfig.Model == "" {
		return fmt.Errorf("%s", i18n.T("app_ai_config_incomplete"))
	}

	uiLang := a.GetLanguage()
	langPrompt := "请用中文回复"
	if uiLang == "en" {
		langPrompt = "Please reply in English"
	}

	// Determine system prompt based on mode
	var systemPrompt string
	switch mode {
	case "generate":
		systemPrompt = ai.TemplateGenerationSystemPrompt + "\n\n" + langPrompt

	case "recommend":
		localTemplates, _ := redc.ListLocalTemplates()
		templateList := make([]string, 0, len(localTemplates))
		for _, t := range localTemplates {
			templateList = append(templateList, fmt.Sprintf("- %s: %s", t.Name, t.Description))
		}
		systemPrompt = fmt.Sprintf(ai.TemplateRecommendationSystemPrompt,
			strings.Join(templateList, "\n"),
			langPrompt)

	case "cost":
		systemPrompt = fmt.Sprintf(ai.CostOptimizationSystemPrompt, langPrompt)
		// Gather running cases info and prepend to the last user message
		casesInfo, runningCount := a.gatherRunningCasesInfo()
		if runningCount > 0 {
			userPrompt := fmt.Sprintf(ai.CostOptimizationUserPrompt, runningCount, casesInfo)
			// Prepend context to the last user message
			if len(messages) > 0 {
				lastIdx := len(messages) - 1
				messages[lastIdx].Content = userPrompt + "\n\n用户额外说明：" + messages[lastIdx].Content
			}
		}

	case "free":
		systemPrompt = fmt.Sprintf(ai.FreeChatSystemPrompt, langPrompt)

	default:
		systemPrompt = fmt.Sprintf(ai.FreeChatSystemPrompt, langPrompt)
	}

	// Build ai.Message slice: system prompt + user-provided history
	aiMessages := make([]ai.Message, 0, len(messages)+1)
	aiMessages = append(aiMessages, ai.Message{Role: "system", Content: systemPrompt})
	for _, m := range messages {
		aiMessages = append(aiMessages, ai.Message{Role: m.Role, Content: m.Content})
	}

	client := ai.NewClient(aiConfig.Provider, aiConfig.APIKey, aiConfig.BaseURL, aiConfig.Model)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err = client.ChatStream(ctx, aiMessages, func(chunk string) error {
		runtime.EventsEmit(a.ctx, "ai-chat-chunk", map[string]string{
			"conversationId": conversationId,
			"chunk":          chunk,
		})
		return nil
	})

	if err != nil {
		runtime.EventsEmit(a.ctx, "ai-chat-complete", map[string]interface{}{
			"conversationId": conversationId,
			"success":        false,
		})
		return fmt.Errorf(i18n.Tf("app_ai_analysis_failed", err))
	}

	runtime.EventsEmit(a.ctx, "ai-chat-complete", map[string]interface{}{
		"conversationId": conversationId,
		"success":        true,
	})
	return nil
}

// gatherRunningCasesInfo collects information about running cases for cost optimization
func (a *App) gatherRunningCasesInfo() (string, int) {
	a.mu.Lock()
	project := a.project
	pricingService := a.pricingService
	costCalculator := a.costCalculator
	a.mu.Unlock()

	if project == nil || pricingService == nil || costCalculator == nil {
		return "", 0
	}

	cases, err := redc.LoadProjectCases(project.ProjectName)
	if err != nil {
		return "", 0
	}

	var caseInfoList []string
	runningCount := 0

	for _, c := range cases {
		if c.State != redc.StateRunning {
			continue
		}
		runningCount++

		if c.Path == "" {
			caseInfo := fmt.Sprintf(`- **%s**
  - 模板: %s
  - 状态: 运行中
  - 说明: 场景路径为空
  - 建议: 请检查场景配置`, c.Name, c.Module)
			caseInfoList = append(caseInfoList, caseInfo)
			continue
		}

		state, err := redc.TfStatus(c.Path)
		if err != nil {
			caseInfo := fmt.Sprintf(`- **%s**
  - 模板: %s
  - 状态: 运行中
  - 说明: 状态获取失败 (%v)
  - 建议: 请检查 Terraform 是否正确安装，场景是否已完成部署`, c.Name, c.Module, err)
			caseInfoList = append(caseInfoList, caseInfo)
			continue
		}

		if state == nil || state.Values == nil {
			caseInfo := fmt.Sprintf(`- **%s**
  - 模板: %s
  - 状态: 运行中
  - 说明: 状态数据为空
  - 建议: 该场景可能尚未创建资源`, c.Name, c.Module)
			caseInfoList = append(caseInfoList, caseInfo)
			continue
		}

		resources := extractResourcesFromState(state)
		if resources == nil || len(resources.Resources) == 0 {
			caseInfo := fmt.Sprintf(`- **%s**
  - 模板: %s
  - 状态: 运行中
  - 说明: 未找到资源信息
  - 建议: 该场景可能尚未创建资源，或资源已被销毁`, c.Name, c.Module)
			caseInfoList = append(caseInfoList, caseInfo)
			continue
		}

		estimate, err := costCalculator.CalculateCost(resources, pricingService)
		if err != nil {
			var resourceList []string
			for _, r := range resources.Resources {
				resourceList = append(resourceList, fmt.Sprintf("  - %s (%s)", r.Name, r.Type))
			}
			caseInfo := fmt.Sprintf(`- **%s**
  - 模板: %s
  - 状态: 运行中
  - 资源数量: %d
  - 资源列表:
%s
  - 说明: 成本计算失败 (%v)
  - 建议: 请检查定价数据是否可用`, c.Name, c.Module, len(resources.Resources), strings.Join(resourceList, "\n"), err)
			caseInfoList = append(caseInfoList, caseInfo)
			continue
		}

		var resourceDetails []string
		for _, rb := range estimate.Breakdown {
			if rb.TotalMonthly > 0 {
				resourceDetails = append(resourceDetails, fmt.Sprintf("  - %s (%s): ¥%.2f/月",
					rb.ResourceName, rb.ResourceType, rb.TotalMonthly))
			} else if !rb.Available {
				resourceDetails = append(resourceDetails, fmt.Sprintf("  - %s (%s): 定价不可用",
					rb.ResourceName, rb.ResourceType))
			}
		}

		provider := "未知"
		if len(estimate.Breakdown) > 0 {
			provider = estimate.Breakdown[0].Provider
		}

		caseInfo := fmt.Sprintf(`- **%s**
  - 模板: %s
  - 云服务商: %s
  - 月度成本: ¥%.2f
  - 资源数量: %d
  - 资源详情:
%s`, c.Name, c.Module, provider, estimate.TotalMonthlyCost, len(estimate.Breakdown), strings.Join(resourceDetails, "\n"))

		caseInfoList = append(caseInfoList, caseInfo)
	}

	return strings.Join(caseInfoList, "\n\n"), runningCount
}
