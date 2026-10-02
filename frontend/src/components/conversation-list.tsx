// Public fields returned by GET /conversations.
export type Conversation = {
  id: number;
  title: string | null;
  created_at: string;
};

// Props describe the data this component receives from its parent.
type ConversationListProps = {
  conversations: Conversation[];
};

export default function ConversationList({
  conversations,
}: ConversationListProps) {
  // 1. HANDLE AN EMPTY LIST
  if (conversations.length === 0) {
    return <p>You are not part of any conversations yet.</p>;
  }

  // 2. DISPLAY CONVERSATIONS IN THE ORDER PROVIDED
  return (
    <ul>
      {conversations.map((conversation) => (
        <li key={conversation.id}>
          {conversation.title ?? `Conversation #${conversation.id}`}
        </li>
      ))}
    </ul>
  );
}
