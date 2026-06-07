package codetools

import "tinychain/mcp"

func (c *CodeTools) Tools() []mcp.Tool {
	return []mcp.Tool{
		c.workspaceInfoTool(),
		c.listDirTool(),
		c.fileInfoTool(),
		c.readFileTool(),
		c.writeFileTool(),
		c.editFileTool(),
		c.makeDirTool(),
		c.deletePathTool(),
		c.searchFilesTool(),
		c.searchTextTool(),
		c.runCommandTool(),
		c.commandStatusTool(),
		c.commandOutputTool(),
		c.commandKillTool(),
	}
}
