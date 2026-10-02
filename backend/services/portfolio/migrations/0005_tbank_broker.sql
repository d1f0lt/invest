-- Т-Банк in the broker list as a placeholder: the app shows it next to
-- Sber and accepts uploads, but the parser has no "tbank" parser yet, so
-- every import fails with "Отчёты этого брокера пока не поддерживаются".
INSERT INTO brokers (id, name, file_formats, icon_url, color, sort_order)
VALUES ('tbank', 'Т-Инвестиции', ARRAY['xlsx'], '/static/brokers/tbank.png', '#FFDD2D', 20)
ON CONFLICT (id) DO NOTHING;
