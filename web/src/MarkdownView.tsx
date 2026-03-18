import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type Props = {
  content: string;
  emptyText?: string;
};

export function MarkdownView({ content, emptyText = "No content." }: Props) {
  const trimmed = content.trim();
  if (!trimmed) {
    return <p className="meta">{emptyText}</p>;
  }
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ ...props }) => <a {...props} target="_blank" rel="noreferrer" />
        }}
      >
        {trimmed}
      </ReactMarkdown>
    </div>
  );
}
