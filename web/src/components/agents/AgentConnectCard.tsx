import { useState } from 'react'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { InfoTip } from '../kit'
import {
  AGENT_MODES,
  AGENT_TOKEN_ENV,
  buildMcpConfig,
  type AgentModeId,
} from '../../lib/agentConfig'

export function AgentConnectCard({
  serverKey,
  binary,
  apiURL,
}: {
  serverKey: string
  binary: string
  apiURL: string
}) {
  const [mode, setMode] = useState<AgentModeId>('agent-core')
  const [copied, setCopied] = useState(false)
  const snippet = buildMcpConfig({ serverKey, binary, mode, apiURL })

  function copySnippet() {
    void navigator.clipboard.writeText(snippet).then(() => {
      setCopied(true)
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-1">
          Connect an AI agent
          <InfoTip label="About connecting an agent">
            Save this as .mcp.json in your project, or run init in the project
            to write it for you. The token is read from the{' '}
            <code>{AGENT_TOKEN_ENV}</code> environment variable, so it never
            lands in the file.
          </InfoTip>
        </CardTitle>
        <CardDescription>
          Set <code className="text-foreground">{AGENT_TOKEN_ENV}</code> to a
          token from below, then add this to your agent&apos;s MCP config.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">Mode</span>
          <Select
            value={mode}
            onValueChange={(value) => {
              const next = AGENT_MODES.find((m) => m.id === value)
              if (next) setMode(next.id)
            }}
          >
            <SelectTrigger aria-label="Tool mode" className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {AGENT_MODES.map((m) => (
                <SelectItem key={m.id} value={m.id}>
                  {m.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="relative">
          <pre
            aria-label="MCP config"
            className="overflow-x-auto rounded-lg bg-muted/50 p-3 pr-20 text-xs"
          >
            <code>{snippet}</code>
          </pre>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="absolute top-2 right-2"
            onClick={copySnippet}
          >
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? 'Copied' : 'Copy'}
          </Button>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Mode</TableHead>
              <TableHead>Tools</TableHead>
              <TableHead>Est. tokens</TableHead>
              <TableHead>Use it for</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {AGENT_MODES.map((m) => (
              <TableRow key={m.id}>
                <TableCell className="font-medium whitespace-normal text-foreground">
                  {m.label}
                </TableCell>
                <TableCell className="whitespace-normal">{m.tools}</TableCell>
                <TableCell className="whitespace-normal">{m.tokens}</TableCell>
                <TableCell className="whitespace-normal text-muted-foreground">
                  {m.hint}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
