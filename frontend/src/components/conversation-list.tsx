// Public fields returned by GET /conversations.
export type Conversation = {
  id: number;
  title: string | null;
  created_at: string;
};

type ConversationListProps = {
  conversations: Conversation[];
  selectedConversationId: number | null;
  onSelect: (conversationId: number) => void;
};

export default function ConversationList({
  conversations,
  selectedConversationId,
  onSelect,
}: ConversationListProps) {
  // 1. HANDLE AN EMPTY LIST
  if (conversations.length === 0) {
    return <p>You are not part of any conversations yet.</p>;
  }

  // 2. LET THE PARENT KNOW WHICH CONVERSATION WAS SELECTED
  return (
    <ul>
      {conversations.map((conversation) => {
        const isSelected = conversation.id === selectedConversationId;

        return (
          <li key={conversation.id}>
            <button
              type="button"
              onClick={() => onSelect(conversation.id)}
              aria-pressed={isSelected}
            >
              {conversation.title ?? `Conversation #${conversation.id}`}
              {isSelected && " — Selected"}
            </button>
          </li>
        );
      })}
    </ul>
  );
}
