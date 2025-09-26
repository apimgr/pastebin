// Middleware to add user context from session to all requests
const addUserContext = (req, res, next) => {
  // Add user data to all requests for template rendering
  res.locals.currentUser = req.session.user || null;
  res.locals.isAuthenticated = !!req.session.user;
  
  // Also add to req for route handlers
  if (req.session.user) {
    req.user = req.session.user;
    req.userToken = req.session.token;
  }
  
  next();
};

module.exports = { addUserContext };