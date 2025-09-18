const express = require('express');

const router = express.Router();

router.get('/', async (req, res) => {
  try {
    const { getModels } = require('../models');
    const models = getModels();
    const page = parseInt(req.query.page) || 1;
    const limit = Math.min(parseInt(req.query.limit) || 50, 100);
    const offset = (page - 1) * limit;
    const now = new Date();

    let pastes, total;
    if (models.Paste.findAndCountAll) {
      const { Op } = models.sequelize.Sequelize;
      const result = await models.Paste.findAndCountAll({
        where: {
          isPublic: true,
          [Op.or]: [
            { expiresAt: null },
            { expiresAt: { [Op.gt]: now } }
          ]
        },
        attributes: ['id', 'title', 'language', 'createdAt', 'views'],
        order: [['createdAt', 'DESC']],
        limit,
        offset
      });
      pastes = result.rows;
      total = result.count;
    } else {
      const query = {
        isPublic: true,
        $or: [
          { expiresAt: null },
          { expiresAt: { $gt: now } }
        ]
      };
      
      total = await models.Paste.countDocuments(query);
      pastes = await models.Paste.find(query)
        .select('id title language createdAt views')
        .sort({ createdAt: -1 })
        .limit(limit)
        .skip(offset);
    }

    const totalPages = Math.ceil(total / limit);

    res.render('recent', {
      siteTitle: process.env.SITE_TITLE || 'Pastebin',
      pastes,
      total,
      page,
      limit,
      totalPages
    });
  } catch (error) {
    console.error('Recent pastes error:', error);
    res.status(500).send('Internal server error');
  }
});

module.exports = router;