CREATE TABLE IF NOT EXISTS feed_comments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS feed_comments_post_id ON feed_comments(post_id, id DESC);
CREATE INDEX IF NOT EXISTS feed_comments_author_id ON feed_comments(author_id);

CREATE TABLE IF NOT EXISTS feed_reactions (
    post_id BIGINT NOT NULL REFERENCES feed_posts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value SMALLINT NOT NULL CHECK (value IN (-1, 1)),
    PRIMARY KEY (post_id, user_id)
);

CREATE INDEX IF NOT EXISTS feed_reactions_user_id ON feed_reactions(user_id);
