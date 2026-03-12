package cmd

import (
	"fmt"
	"red-cloud/i18n"
	redc "red-cloud/mod"
	"red-cloud/mod/gologger"

	"github.com/spf13/cobra"
)

var changeConfig redc.ChangeCommand

// helper: 通用的执行器
func runAction(actionType string, caseID string) {
	c, err := redcProject.GetCase(caseID)
	if err != nil {
		if IsJSON() {
			PrintJSONError(fmt.Errorf("%s", i18n.Tf("action_case_not_found", caseID, err)))
			return
		}
		gologger.Error().Msgf(i18n.Tf("action_case_not_found", caseID, err))
		return
	}

	redc.RedcLog(fmt.Sprintf("Action %s on %s", actionType, caseID))

	var actionErr error
	switch actionType {
	case "stop":
		actionErr = c.Stop()
	case "start":
		actionErr = c.TfApply()
	case "kill":
		actionErr = c.Kill()
	case "change":
		actionErr = c.Change(changeConfig)
	case "status":
		if IsJSON() {
			runStatusJSON(c)
			return
		}
		actionErr = c.Status()
	case "rm":
		actionErr = c.Remove()
	}

	if actionErr != nil {
		if IsJSON() {
			PrintJSONError(actionErr)
			return
		}
		gologger.Error().Msgf(i18n.Tf("action_failed", actionType, actionErr))
	} else {
		if IsJSON() {
			PrintJSON(map[string]string{
				"action": actionType,
				"case":   c.Name,
				"id":     c.GetId(),
				"state":  string(c.State),
			})
			return
		}
		gologger.Info().Msgf(i18n.Tf("action_success", actionType, c.Name, c.GetId()))
	}
}

// runStatusJSON outputs case status as JSON
func runStatusJSON(c *redc.Case) {
	result := map[string]interface{}{
		"name":  c.Name,
		"id":    c.Id,
		"state": string(c.State),
	}
	state, err := redc.TfStatus(c.Path)
	if err != nil {
		PrintJSONError(err)
		return
	}
	if state.Values != nil {
		outputs := map[string]interface{}{}
		for k, v := range state.Values.Outputs {
			outputs[k] = v.Value
		}
		result["outputs"] = outputs

		if state.Values.RootModule != nil {
			var resources []map[string]string
			for _, res := range state.Values.RootModule.Resources {
				resources = append(resources, map[string]string{
					"type":    res.Type,
					"address": res.Address,
					"name":    res.Name,
				})
			}
			result["resources"] = resources
		}
	}
	PrintJSON(result)
}

// 定义各个命令
var stopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: i18n.T("stop_short"),
	Run: func(cmd *cobra.Command, args []string) {
		runAction("stop", args[0])
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [id]",
	Short: i18n.T("status_short"),
	Run: func(cmd *cobra.Command, args []string) {
		runAction("status", args[0])
	},
}

var changeCmd = &cobra.Command{
	Use:   "change [id]",
	Short: i18n.T("change_short"),
	Run: func(cmd *cobra.Command, args []string) {
		runAction("change", args[0])
	},
}

var startCmd = &cobra.Command{
	Use:   "start [id]",
	Short: i18n.T("start_short"),
	Run: func(cmd *cobra.Command, args []string) {
		runAction("start", args[0])
	},
}

var killCmd = &cobra.Command{
	Use:   "kill [id]",
	Short: i18n.T("kill_short"),
	Run: func(cmd *cobra.Command, args []string) {
		runAction("kill", args[0])
	},
}
var rmCmd = &cobra.Command{
	Use:   "rm [id]",
	Short: i18n.T("rm_short"),
	Run: func(cmd *cobra.Command, args []string) {
		runAction("rm", args[0])
	},
}

var listCmd = &cobra.Command{
	Use:   "ps",
	Short: i18n.T("ps_short"),
	Run: func(cmd *cobra.Command, args []string) {
		if IsJSON() {
			cases, err := redc.LoadProjectCases(redc.Project)
			if err != nil {
				PrintJSONError(err)
				return
			}
			PrintJSON(cases)
			return
		}
		redcProject.CaseList()
	},
}

// 注册命令
func init() {
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(changeCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(rmCmd)
	changeCmd.Flags().BoolVar(&changeConfig.IsRemove, "rm", false, i18n.T("flag_change_rm"))
}
