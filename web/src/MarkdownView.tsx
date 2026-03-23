import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type Props = {
  content: string;
  emptyText?: string;
  githubBlobBase?: string;
  repoPath?: string;
  worktreePath?: string;
};

function stripKnownRepoPrefix(href: string, repoPath: string | undefined, worktreePath: string | undefined): string {
  const prefixes = [worktreePath, repoPath].filter((value): value is string => Boolean(value)).map((value) => value.replace(/\/+$/, ""));
  for (const prefix of prefixes) {
    if (href === prefix) {
      return ".";
    }
    if (href.startsWith(`${prefix}/`)) {
      return href.slice(prefix.length + 1);
    }
  }
  return href;
}

function rewriteMarkdownHref(
  href: string | undefined,
  githubBlobBase: string | undefined,
  repoPath: string | undefined,
  worktreePath: string | undefined
): string | undefined {
  if (!href) {
    return href;
  }
  const normalizedHrefInput = stripKnownRepoPrefix(href, repoPath, worktreePath);
  const normalizedHref = normalizedHrefInput.replace(/^\.\/+/, "");
  if (/^[a-z]+:/i.test(href) || href.startsWith("//") || href.startsWith("#")) {
    return href;
  }
  if (!githubBlobBase) {
    return normalizedHrefInput;
  }
  const normalizedBase = githubBlobBase.replace(/\/+$/, "");
  return `${normalizedBase}/${normalizedHref}`;
}

export function MarkdownView({ content, emptyText = "No content.", githubBlobBase, repoPath, worktreePath }: Props) {
  const trimmed = content.trim();
  if (!trimmed) {
    return <p className="meta">{emptyText}</p>;
  }
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ href, ...props }) => (
            <a
              {...props}
              href={rewriteMarkdownHref(href, githubBlobBase, repoPath, worktreePath)}
              target="_blank"
              rel="noreferrer"
            />
          )
        }}
      >
        {trimmed}
      </ReactMarkdown>
    </div>
  );
}
